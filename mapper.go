package main

import (
    "database/sql"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "io"
    "io/ioutil"
    re "regexp"
    "strconv"
    "os"
    "os/exec"
    "sort"
    s "strings"
    "sync"
    "jfb/svgxml"
    _ "github.com/lib/pq"
)

type Config struct {
    Colours map[string]string `json:"colours"`
}

type DbConfig struct {
    Server      map[string]string       `json:"db_server"`
    Creds       map[string]string       `json:"db_creds"`
    Schema      map[string]string       `json:"db_schema"`
}

// set up integer array sorting
type IntArray []int
func (list IntArray) Len() int          { return len(list) }
func (list IntArray) Swap(a, b int)     { list[a], list[b] = list[b], list[a] }
func (list IntArray) Less(a, b int) bool { return list[a] < list[b] }

// suck in count data
func db_data() (map[string]int, map[string]int) {
    var dbconfig        DbConfig

    jsoncfg, err := ioutil.ReadFile("dbconfig.json")
    if err != nil {
        panic(err)
    }

    err = json.Unmarshal(jsoncfg, &dbconfig)
    if err != nil {
        fmt.Fprintf(os.Stderr, "DB config unmarshal: ")
        panic(err)
    }

    state_counts :=     make(map[string]int)
    county_counts :=    make(map[string]int)

    dbh, err := sql.Open(dbconfig.Server["dbtype"],
        dbconfig.Server["dbtype"] + "://" + dbconfig.Creds["username"] + ":" +
        dbconfig.Creds["password"] + "@" + dbconfig.Server["dbhost"] + "/" +
        dbconfig.Server["dbname"] + dbconfig.Server["dbopts"])
    if err != nil {
        log.Fatal(err)
    }

    query :=
            "select " +
                dbconfig.Schema["state_column"] + ", " +
                dbconfig.Schema["county_column"] + ", " +
                dbconfig.Schema["tally_column"] +
            "from " +
                dbconfig.Schema["tables"] + " " +
            dbconfig.Schema["where"] + " " +
            dbconfig.Schema["group_by"]
    rows, err := dbh.Query(query)
    // fmt.Println(query)
    if err != nil {
        log.Fatal(err)
    }

    defer rows.Close()
    for rows.Next() {
        var state, county string
        var count int
        if err := rows.Scan(&state, &county, &count); err != nil {
            log.Fatal(err)
        }
        state_counts[state] += count
        state_county_key := s.Replace(state + " " + county, " ", "_", -1)
        county_counts[state_county_key] = count
    }
    if err := rows.Err(); err != nil {
        log.Fatal(err)
    }

    return state_counts, county_counts

}

func usage(exit_status int) {
    flag.PrintDefaults()
    fmt.Fprintf(os.Stderr, "\n  -state and -county may be specified multiple times\n\nAdditional parameters: none\n")
    os.Exit(exit_status)
}

func colour_svgdata(mapsvg []byte, data map[string]int, re_fill *re.Regexp, colours map[string]string, mincount []int) (string) {
    mapsvg_obj := svgxml.XML2SVG(mapsvg)
    if mapsvg_obj == nil {
        return ":SVGERR"
    }

    for id, count := range data {
        for _, mc := range mincount {
            if count >= mc {
                element := svgxml.FindPathById(mapsvg_obj, id)
                if element != nil {
                    element.Style = string(re_fill.ReplaceAll([]byte(element.Style), []byte("${1}" + colours[strconv.Itoa(mc)])))
                } else {
                    fmt.Fprintf(os.Stderr, "'%s' not found\n", id)
                }
            }
        }
    }
    return string(svgxml.SVG2XML(mapsvg_obj, true))
}

func main() {

    var config  Config
    var wg      sync.WaitGroup

    h := flag.Bool("h", false, "usage information")
    help := flag.Bool("help", false, "usage information")
    state_flag := flag.String("state", "", "state map(s), format input_filename:output_filename")
    county_flag := flag.String("county", "", "county map, format input_filename:output_filename")
    flag.Parse()

    if *h || *help {
        usage(0)
    }

    if (*state_flag == "" && *county_flag == "") || flag.NArg() > 0 {
        usage(1)
    }

    // 'flag' package is just used for syntax-checking. here we get the actual
    // data from the command line

    state_map := make([]string, s.Count(s.Join(os.Args, " "), "-state"))
    county_map := make([]string, s.Count(s.Join(os.Args, " "), "-county"))
    nstates := 0
    ncounties := 0
    for arg_i, arg := range os.Args {
        if arg == "-state" {
            state_map[nstates] = os.Args[arg_i+1]
            nstates++
        } else if arg == "-county" {
            county_map[ncounties] = os.Args[arg_i+1]
            ncounties++
        }
    }

    jsoncfg, err := ioutil.ReadFile("config.json")
    if err != nil {
        panic(err)
    }

    err = json.Unmarshal(jsoncfg, &config)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Config unmarshal: ")
        panic(err)
    }

    // make sorted list of keys (minimum counts) for later comparisons
    mincount := make([]int, len(config.Colours))
    i := 0
    for k, _ := range config.Colours {
        k_i, _ := strconv.ParseInt(k, 0, 64)
        mincount[i] = int(k_i)
        i++
    }

    sort.Sort(IntArray(mincount))

    re_fill, err := re.Compile(`(fill:#)......`)
    if err != nil {
        panic(err)
    }

    re_svgext, err := re.Compile(`\.svg$`)
    if err != nil {
        panic(err)
    }

    state_data, county_data := db_data()

    // fmt.Println(state_data, county_data)

    for _, datatype := range [2]string{"state", "county"} {
        var maps []string
        var data map[string]int

        if datatype == "state" {
            maps = state_map
            data = state_data
        } else {
            maps = county_map
            data = county_data
        }

        for _, mapset := range maps {
            wg.Add(1)
            go func(filenames string) {

                defer wg.Done()
                // fn = [infile, outfile]
                fn := s.Split(filenames, ":")
                mapsvg, err := ioutil.ReadFile(fn[0])
                if err != nil {
                    fmt.Fprintf(os.Stderr, "can't read '" + fn[0] + "': " + err.Error())
                    return
                }
                svg_coloured := colour_svgdata(mapsvg, data, re_fill, config.Colours, mincount)
                if svg_coloured == ":SVGERR" {
                    fmt.Fprintf(os.Stderr, "can't create SVG object from " + fn[0])
                    return
                }
                ret := re_svgext.Find([]byte(fn[1]))
                if ret == nil {
                    // going to call ImageMagick's 'convert' because I can't find
                    // a damn SVG package that can write to a non-SVG image and I
                    // don't have the chops to write one.
                    cmd := exec.Command("convert", "svg:-", fn[1])
                    convert_stdin, err := cmd.StdinPipe()
                    if err != nil {
                        log.Fatal(err)
                    }
                    go func() {
                        defer convert_stdin.Close()
                        io.WriteString(convert_stdin, svg_coloured)
                    }()
                    _, err = cmd.CombinedOutput()
                    if err != nil {
                        log.Fatal(err)
                    }
                } else {
                    // just going back to an SVG file
                    err := ioutil.WriteFile(fn[1], []byte(svg_coloured), 0666)
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "can't write to '" + fn[1] + "': " + err.Error())
                        return
                    }
                }

            }(mapset)
        }
    }

    wg.Wait()

    // fmt.Println(string(svgxml.SVG2XML(mapsvg_obj, true)))

}

// ex:ai:sw=4:ts=8:
