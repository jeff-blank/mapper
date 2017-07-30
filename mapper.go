package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "io/ioutil"
    "regexp"
    "strconv"
    "os"
    "sort"
    s "strings"
    "jfb/svgxml"
    _ "github.com/lib/pq"
)

type Config struct {
    Colours map[string]string `json:"colours"`
}

type DbConfig struct {
    Server      map[string]string       `json:"db_server"`
    Creds       map[string]string       `json:"db_creds"`
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

    dbh, err := sql.Open("postgres",
        "postgres://" + dbconfig.Creds["username"] + ":" + dbconfig.Creds["password"] + "@" +
        dbconfig.Server["dbhost"] + "/" + dbconfig.Server["dbname"] + "?sslmode=require")
    if err != nil {
        log.Fatal(err)
    }

    rows, err := dbh.Query(`SELECT state, county, count(county) from hits where ` +
        `country = 'US' group by state, county`)
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

func main() {

    var config Config

    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "Please specify at least one filename on the command line.\n")
        os.Exit(1)
    }

    jsoncfg, err := ioutil.ReadFile("config.json")
    if err != nil {
        panic(err)
    }

    mapsvg, err := ioutil.ReadFile(os.Args[1])
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

    mapsvg_obj := svgxml.XML2SVG(mapsvg)
    if mapsvg_obj == nil {
        os.Exit(1)
    }

    re_fill, err := regexp.Compile(`(fill:#)......`)
    if err != nil {
        panic(err)
    }

    state, _ := db_data()

    for state, state_count := range state {
	for _, mc := range mincount {
	    if state_count >= mc {
		element := svgxml.FindPathById(mapsvg_obj, state)
		if element != nil {
		    element.Style = string(re_fill.ReplaceAll([]byte(element.Style), []byte("${1}" + config.Colours[strconv.Itoa(mc)])))
		} else {
		    fmt.Fprintf(os.Stderr, "'%s' not found\n", state)
		}

	    }
	}
    }


    fmt.Println(string(svgxml.SVG2XML(mapsvg_obj, true)))

}

// vim:ai:sw=4:ts=8:
