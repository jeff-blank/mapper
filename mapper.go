package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "image"
    "image/color"
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
    LADefaults  map[string]string                       `json:"legend_annotation_defaults"`
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

func annotate(img *image.RGBA, legend_anno_dfl, attrs map[string]string, data map[string]int) {

    ann_x_str       := legend_anno_dfl["annotation_x"]
    ann_y_str       := legend_anno_dfl["annotation_y"]
    ann_timefmt     := legend_anno_dfl["annotation_timefmt"]
    ann_fontfile    := legend_anno_dfl["annotation_fontfile"]
    ann_fontsz_str  := legend_anno_dfl["annotation_sz"]
    ann_str         := legend_anno_dfl["annotation_str"]

    if len(attrs["annotation_x"]) > 0 {
        ann_x_str = attrs["annotation_x"]
    }
    if len(attrs["annotation_y"]) > 0 {
        ann_y_str = attrs["annotation_y"]
    }
    ann_x, err := strconv.Atoi(ann_x_str)
    ann_y, err := strconv.Atoi(ann_y_str)

    if len(attrs["annotation_fontfile"]) > 0 {
        ann_fontfile = attrs["annotation_fontfile"]
    }

    if len(attrs["annotation_fontsize"]) > 0 {
        ann_fontsz_str = attrs["annotation_sz"]
    }
    fontsize, _ := strconv.ParseFloat(ann_fontsz_str, 64)

    if len(attrs["annotation_timefmt"]) > 0 {
        ann_timefmt = attrs["annotation_timefmt"]
    }

    if len(attrs["annotation_str"]) > 0 {
        ann_str = attrs["annotation_str"]
    }

    ann_fontdata, err := ioutil.ReadFile(ann_fontfile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "can't read from annotation font file '%s': %s\n", ann_fontfile, err.Error())
        os.Exit(1)
    }
    font, err := freetype.ParseFont(ann_fontdata)
    if err != nil {
        log.Fatal(err)
    }

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

    annotation := s.Replace(ann_str, "%t%", strconv.Itoa(total_hits), -1)
    annotation = s.Replace(annotation, "%c%", strconv.Itoa(regions), -1)
    if s.Index(annotation, "%T%") >= 0 {
        annotation = s.Replace(annotation, "%T%", time.Now().Format(ann_timefmt), -1)
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

func ah_hates_legends(img *image.RGBA, mincount []int, colours, defaults, attrs map[string]string) {
    fontfile := defaults["legend_fontfile"]
    fontsize_str := defaults["legend_fontsize"]
    gravity := defaults["legend_gravity"]
    orient := defaults["legend_orient"]
    cell_w_str := defaults["legend_cell_width"]
    cell_h_str := defaults["legend_cell_height"]
    cell_gap_str := defaults["legend_cell_gap"]

    if len(attrs["legend_fontfile"]) > 0 {
        fontfile = attrs["legend_fontfile"]
    }

    if len(attrs["legend_fontsize"]) > 0 {
        fontsize_str = attrs["legend_fontsize"]
    }
    fontsize, _ := strconv.ParseFloat(fontsize_str, 64)

    if len(attrs["legend_gravity"]) > 0 {
        gravity = attrs["legend_gravity"]
    }

    if len(attrs["legend_orient"]) > 0 {
        orient = attrs["legend_orient"]
    }

    if len(attrs["legend_cell_width"]) > 0 {
        cell_w_str = attrs["legend_cell_width"]
    }
    cell_w, _ := strconv.Atoi(cell_w_str)
    if len(attrs["legend_cell_height"]) > 0 {
        cell_h_str = attrs["legend_cell_height"]
    }
    cell_h, _ := strconv.Atoi(cell_h_str)
    if len(attrs["legend_cell_gap"]) > 0 {
        cell_gap_str = attrs["legend_cell_gap"]
    }
    cell_gap, _ := strconv.Atoi(cell_gap_str)

    fontdata, err := ioutil.ReadFile(fontfile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "can't read from legend font file '%s': %s\n", fontfile, err.Error())
        os.Exit(1)
    }
    font, err := freetype.ParseFont(fontdata)
    if err != nil {
        log.Fatal(err)
    }
    b := img.Bounds()
    ft_ctx := freetype.NewContext()
    ft_ctx.SetDPI(72.0)
    ft_ctx.SetFont(font)
    ft_ctx.SetFontSize(fontsize)
    ft_ctx.SetClip(b)
    ft_ctx.SetDst(img)
    ft_ctx.SetSrc(image.Black)

    legend_width := cell_w
    legend_height := cell_h
    if orient == "vertical" {
        legend_height = len(colours) * (cell_h + cell_gap) - cell_gap
    } else {
        legend_width = len(colours) * (cell_w + cell_gap) - cell_gap
    }

    box_x := 0
    box_y := 0
    if s.ToLower(gravity)[0] == 's' {
        box_y = b.Dy() - legend_height
    }
    if s.ToLower(gravity)[1] == 'e' {
        box_x = b.Dx() - legend_width
    }

    for i, mc := range mincount {
        c_red, _ := strconv.ParseUint(colours[strconv.Itoa(mc)][0:2], 16, 8)
        c_green, _ := strconv.ParseUint(colours[strconv.Itoa(mc)][2:4], 16, 8)
        c_blue, _ := strconv.ParseUint(colours[strconv.Itoa(mc)][4:6], 16, 8)
        fill := color.RGBA{uint8(c_red), uint8(c_green), uint8(c_blue), 255}
        draw.Draw(img, image.Rect(box_x, box_y, box_x + cell_w, box_y + cell_h),
                    &image.Uniform{fill}, image.ZP, draw.Src)
        if orient == "vertical" {
            box_y += cell_h + cell_gap
        } else {
            box_x += cell_w + cell_gap
        }

        label := strconv.Itoa(mc)
        if i == len(mincount) - 1 {
            label = label + "+"
        } else if mincount[i+1] != (mc + 1) {
            label = label + "-" + strconv.Itoa(mincount[i+1] - 1)
        }
        var text_x, text_y int
        if orient == "vertical" {
            text_x = box_x + 4
            text_y = box_y - cell_h + int(ft_ctx.PointToFixed(fontsize) >> 6)
        } else {
            text_x = box_x - cell_w + 4
            text_y = box_y + int(ft_ctx.PointToFixed(fontsize) >> 6)
        }
        fpt := freetype.Pt(text_x, text_y)
        _, err = ft_ctx.DrawString(label, fpt)
        if err != nil {
            log.Fatal(err)
        }
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

                    ah_hates_legends(img_rgba, mincount, config.Colours, config.LADefaults, attrs)

                    annotate(img_rgba, config.LADefaults, attrs, mapdata)
                    outfile_handle, err := os.Create(attrs["outfile"])
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "can't create '" + attrs["outfile"] + "': " + err.Error())
                        return
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
