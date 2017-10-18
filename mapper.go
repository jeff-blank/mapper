package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "image"
    "image/draw"
    "image/png"
    "io"
    "io/ioutil"
    "log"
    "os"
    "os/exec"
    re "regexp"
    "sort"
    "strconv"
    s "strings"
    "sync"
    "time"
    "jfb/svgxml"
    "github.com/golang/freetype"
    _ "github.com/lib/pq"
)

type Config struct {
    Colours     map[string]string                       `json:"colours"`
    DefaultFont string                                  `json:"annotation_font_default"`
    Maps        map[string]map[string]map[string]string `json:"maps"`
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

func colour_svgdata(mapsvg_obj *svgxml.SVG, data map[string]int, re_fill *re.Regexp, colours map[string]string, mincount []int) (string, []string) {

    var errors []string

    for id, count := range data {
        for _, mc := range mincount {
            if count >= mc {
                element := svgxml.FindPathById(mapsvg_obj, id)
                if element != nil {
                    element.Style = string(re_fill.ReplaceAll([]byte(element.Style), []byte("${1}" + colours[strconv.Itoa(mc)])))
                } else {
                    errors = append(errors, "'" + id + "' not found")
                }
            }
        }
    }
    return string(svgxml.SVG2XML(mapsvg_obj, true)), errors
}

func annotate(img *image.RGBA, fontfile string, attrs map[string]string, data map[string]int) {
    fontdata, err := ioutil.ReadFile(fontfile)
    if err != nil {
        log.Fatal(err)
    }
    font, err := freetype.ParseFont(fontdata)
    if err != nil {
        log.Fatal(err)
    }

    ann_x, _ := strconv.ParseInt(attrs["annotation_x"], 10, 64)
    ann_y, _ := strconv.ParseInt(attrs["annotation_y"], 10, 64)

    fontsize, _ := strconv.ParseFloat(attrs["annotation_sz"], 64)
    ft_ctx := freetype.NewContext()
    ft_ctx.SetDPI(72.0)
    ft_ctx.SetFont(font)
    ft_ctx.SetFontSize(fontsize)
    ft_ctx.SetClip(img.Bounds())
    ft_ctx.SetDst(img)
    ft_ctx.SetSrc(image.Black)
    pt := freetype.Pt(int(ann_x), int(ann_y)+int(ft_ctx.PointToFixed(fontsize) >> 6))

    total_hits := 0;
    for _, hits := range data {
        total_hits += hits
    }
    regions := len(data)
    if len(attrs["regions_adjust"]) > 0 {
        adj, _ := strconv.Atoi(attrs["regions_adjust"])
        regions += adj
    }

    annotation := s.Replace(attrs["annotation"], "%t%", strconv.Itoa(total_hits), -1)
    annotation = s.Replace(annotation, "%c%", strconv.Itoa(regions), -1)
    if s.Index(annotation, "%T%") >= 0 {
        annotation = s.Replace(annotation, "%T%", time.Now().Format(attrs["annotation_timefmt"]), -1)
    }
    ann_lines := s.Split(annotation, "\n")
    for _, line := range ann_lines {
        _, err = ft_ctx.DrawString(line, pt)
        if err != nil {
            log.Fatal(err)
        }
        pt.Y += ft_ctx.PointToFixed(fontsize * 1.5)
    }
}

func main() {

    var config  Config
    var wg      sync.WaitGroup

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

    for maptype, mapset := range config.Maps {
        var data map[string]int

        if maptype == "states" {
            data = state_data
        } else {
            data = county_data
        }

        for infile, attrs := range mapset {
            wg.Add(1)
            // go func(srcfile, dstfile, outsize, maptype string, mapdata map[string]int)
            go func(srcfile string, attrs map[string]string, maptype string, mapdata map[string]int) {

                var county_data_new map[string]int

                defer wg.Done()
                defer os.Stderr.Close()

                mapsvg, err := ioutil.ReadFile(srcfile)
                if err != nil {
                    fmt.Fprintf(os.Stderr, "can't read '" + srcfile + "': " + err.Error())
                    return
                }

                mapsvg_obj := svgxml.XML2SVG(mapsvg)
                if mapsvg_obj == nil {
                    fmt.Fprintf(os.Stderr, "can't create SVG object from " + srcfile)
                    return
                }

                if maptype == "counties" {
                    // This block has the effect of pruning county data for
                    // *states* that don't appear in the given map. This is so
                    // that counties in states outside the map don't cause
                    // error messages and counties in the map that have a
                    // different (incorrect)( name in the data do generate
                    // errors.

                    var map_state_list []string

                    county_data_new = make(map[string]int)

                    // first, make a list of all states in the map using
                    // state_data as the source of state names
                    for _, g := range mapsvg_obj.G {
                        for state, _ := range state_data {
                            if s.Index(g.Id, state + "_") == 0 {
                                map_state_list = append(map_state_list, state + "_")
                            }
                        }
                    }

                    // next, search county names in data for states found in
                    // the map and copy only county data entries for those
                    // found states
                    for state_county, sc_count := range mapdata {
                        found_state := false
                        for _, state_ := range map_state_list {
                            if s.Index(state_county, state_) == 0 {
                                found_state = true
                                break
                            }
                        }
                        if found_state == true {
                            county_data_new[state_county] = sc_count
                        }
                    }

                    // replace the function-local dataset with the pruned data
                    mapdata = county_data_new
                }

                svg_coloured, errlist := colour_svgdata(mapsvg_obj, mapdata, re_fill, config.Colours, mincount)
                if len(errlist) > 0 {
                    for _, errmsg := range errlist {
                        fmt.Fprintf(os.Stderr, "%s: %s\n", srcfile, errmsg)
                    }
                }

                ret := re_svgext.Find([]byte(attrs["outfile"]))
                if ret == nil {
                    // going to call ImageMagick's 'convert' because I can't find
                    // a damn SVG package that can write to a non-SVG image and I
                    // don't have the chops to write one.
                    cmd := exec.Command("convert", "svg:-", "-resize", attrs["outsize"], "png:-")
                    convert_stdin, err := cmd.StdinPipe()
                    if err != nil {
                        log.Fatal(err)
                    }
                    go func() {
                        defer convert_stdin.Close()
                        io.WriteString(convert_stdin, svg_coloured)
                    }()

                    // grab PNG data and cram it into an RGBA image
                    png_data, err := cmd.Output()
                    if err != nil {
                        log.Fatal(err)
                    }
                    png_reader := s.NewReader(string(png_data))
                    img, _, err := image.Decode(png_reader)
                    b := img.Bounds()
                    img_rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
                    draw.Draw(img_rgba, img_rgba.Bounds(), img, b.Min, draw.Src)

                    /*
                    ** do stuff with the image here: annotations, legend, etc
                    */

                    fontfile := config.DefaultFont
                    if len(attrs["annotation_font_override"]) > 0 {
                        fontfile = attrs["annotation_font_override"]
                    }

                    annotate(img_rgba, fontfile, attrs, mapdata)

                    outfile_handle, err := os.Create(attrs["outfile"])
                    if err != nil {
                        log.Fatal(err)
                    }
                    if err := png.Encode(outfile_handle, img_rgba); err != nil {
                        outfile_handle.Close()
                        log.Fatal(err)
                    }
                } else {
                    // just going back to an SVG file
                    err := ioutil.WriteFile(attrs["outfile"], []byte(svg_coloured), 0666)
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "can't write to '" + attrs["outfile"] + "': " + err.Error())
                        return
                    }
                }

            }(infile, attrs, maptype, data)
        }
    }

    wg.Wait()

}

// vim:ts=4:et:
// ex:ai:sw=4:ts=1000:
