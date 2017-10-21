package main

import (
    "database/sql"
    "flag"
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

    "gopkg.in/yaml.v2"
)

type LegendAnnotateParams struct {
    LegendGravity       string  `yaml:"legend_gravity"`
    LegendOrient        string  `yaml:"legend_orient"`
    LegendFontFile      string  `yaml:"legend_fontfile"`
    LegendFontSize      float64 `yaml:"legend_fontsize"`
    LegendCellWidth     int     `yaml:"legend_cell_width"`
    LegendCellHeight    int     `yaml:"legend_cell_height"`
    LegendCellGap       int     `yaml:"legend_cell_gap"`
    AnnotationFontFile  string  `yaml:"annotation_fontfile"`
    AnnotationFontSize  float64 `yaml:"annotation_fontsize"`
    AnnotationTimeFmt   string  `yaml:"annotation_timefmt"`
    AnnotationString    string  `yaml:"annotation_str"`
    AnnotationX         int     `yaml:"annotation_x"`
    AnnotationY         int     `yaml:"annotation_y"`
}

type MapSet struct {
    InputFile           string                  `yaml:"infile"`
    OutputFile          string                  `yaml:"outfile"`
    OutputSize          string                  `yaml:"outsize"`
    RegionAdjustment    int                     `yaml:"regions_adjustment"`
    LegendAnnotate      LegendAnnotateParams    `yaml:",inline"`
    InlineData          map[string]int          `yaml:"inline_data"`
}

type Config struct {
    General     map[string]string
    Colours     map[string]string
    LADefaults  LegendAnnotateParams    `yaml:"legend_annotation_defaults"`
    Maps        map[string][]MapSet     `yaml:"maps"`
    DbParam     map[string]string       `yaml:"database"`
}

// set up integer array sorting
type IntArray []int
func (list IntArray) Len() int          { return len(list) }
func (list IntArray) Swap(a, b int)     { list[a], list[b] = list[b], list[a] }
func (list IntArray) Less(a, b int) bool { return list[a] < list[b] }

// suck in count data
func db_data(dbconfig map[string]string) (map[string]int, map[string]int) {

    state_counts    := make(map[string]int)
    county_counts   := make(map[string]int)

    dbh, err := sql.Open(dbconfig["type"],
                    dbconfig["type"] + "://" + dbconfig["username"] + ":" +
                    dbconfig["password"] + "@" + dbconfig["host"] + "/" +
                    dbconfig["name"] + dbconfig["connect_opts"])
    if err != nil {
        log.Fatal(err)
    }

    query :=
            "select " +
                dbconfig["state_column"] + ", " +
                dbconfig["county_column"] + ", " +
                dbconfig["tally_column"] +
            "from " +
                dbconfig["tables"] + " " +
                dbconfig["where"] + " " +
                dbconfig["group_by"]
    rows, err := dbh.Query(query)
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

func annotate(img *image.RGBA, defaults LegendAnnotateParams, attrs MapSet, data map[string]int) {

    ann_x       := defaults.AnnotationX
    ann_y       := defaults.AnnotationY
    timefmt     := defaults.AnnotationTimeFmt
    fontfile    := defaults.AnnotationFontFile
    fontsize    := defaults.AnnotationFontSize
    ann_str     := defaults.AnnotationString

    if attrs.LegendAnnotate.AnnotationX > 0 {
        ann_x = attrs.LegendAnnotate.AnnotationX
    }
    if attrs.LegendAnnotate.AnnotationY > 0 {
        ann_y = attrs.LegendAnnotate.AnnotationY
    }

    if len(attrs.LegendAnnotate.AnnotationFontFile) > 0 {
        fontfile = attrs.LegendAnnotate.AnnotationFontFile
    }

    if attrs.LegendAnnotate.AnnotationFontSize > 0 {
        fontsize = attrs.LegendAnnotate.AnnotationFontSize
    }

    if len(attrs.LegendAnnotate.AnnotationTimeFmt) > 0 {
        timefmt = attrs.LegendAnnotate.AnnotationTimeFmt
    }

    if len(attrs.LegendAnnotate.AnnotationString) > 0 {
        ann_str = attrs.LegendAnnotate.AnnotationString
    }

    fontdata, err := ioutil.ReadFile(fontfile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "annotate()\n")
        log.Fatal(err)
    }
    font, err := freetype.ParseFont(fontdata)
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
    if attrs.RegionAdjustment != 0 {
        regions += attrs.RegionAdjustment
    }

    annotation := s.Replace(ann_str, "%t%", strconv.Itoa(total_hits), -1)
    annotation = s.Replace(annotation, "%c%", strconv.Itoa(regions), -1)
    if s.Index(annotation, "%T%") >= 0 {
        annotation = s.Replace(annotation, "%T%", time.Now().Format(timefmt), -1)
    }
    ann_lines := s.Split(annotation, "\n")
    for _, line := range ann_lines {
        _, err = ft_ctx.DrawString(line, pt)
        if err != nil {
            log.Fatal(err)
        }
        pt.Y += ft_ctx.PointToFixed(fontsize * 1.2)
    }
}

func ah_hates_legends(img *image.RGBA, mincount []int, colours map[string]string, defaults LegendAnnotateParams, attrs MapSet) {
    fontfile    := defaults.LegendFontFile
    fontsize    := defaults.LegendFontSize
    gravity     := defaults.LegendGravity
    orient      := defaults.LegendOrient
    cell_w      := defaults.LegendCellWidth
    cell_h      := defaults.LegendCellHeight
    cell_gap    := defaults.LegendCellGap

    if len(attrs.LegendAnnotate.LegendFontFile) > 0 {
        fontfile = attrs.LegendAnnotate.LegendFontFile
    }

    if attrs.LegendAnnotate.LegendFontSize > 0 {
        fontsize = attrs.LegendAnnotate.LegendFontSize
    }

    if len(attrs.LegendAnnotate.LegendGravity) > 0 {
        gravity = attrs.LegendAnnotate.LegendGravity
    }

    if len(attrs.LegendAnnotate.LegendOrient) > 0 {
        orient = attrs.LegendAnnotate.LegendOrient
    }

    if attrs.LegendAnnotate.LegendCellWidth > 0 {
        cell_w = attrs.LegendAnnotate.LegendCellWidth
    }
    if attrs.LegendAnnotate.LegendCellHeight > 0 {
        cell_h = attrs.LegendAnnotate.LegendCellHeight
    }
    if attrs.LegendAnnotate.LegendCellGap > 0 {
        cell_gap = attrs.LegendAnnotate.LegendCellGap
    }

    fontdata, err := ioutil.ReadFile(fontfile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ah_hates_legends()\n")
        log.Fatal(err)
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

    config_file := flag.String("conf", "mapper.yml", "configuration file")
    flag.Parse()

    yamlcfg, err := ioutil.ReadFile(*config_file)
    if err != nil {
        log.Fatal(err)
    }

    err = yaml.Unmarshal(yamlcfg, &config)
    if err != nil {
        log.Fatal(err)
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
        log.Fatal(err)
    }

    re_svgext, err := re.Compile(`\.svg$`)
    if err != nil {
        log.Fatal(err)
    }

    state_data, county_data := db_data(config.DbParam)

    for maptype, mapset := range config.Maps {
        var data map[string]int

        if maptype == "states" {
            data = state_data
        } else {
            data = county_data
        }

        for _, attrs := range mapset {
            wg.Add(1)
            go func(attrs MapSet, maptype string, mapdata map[string]int) {

                var county_data_new map[string]int

                defer wg.Done()
                defer os.Stderr.Close()

                mapsvg, err := ioutil.ReadFile(attrs.InputFile)
                if err != nil {
                    fmt.Fprintf(os.Stderr, "can't read '" + attrs.InputFile+ "': " + err.Error())
                    return
                }

                mapsvg_obj := svgxml.XML2SVG(mapsvg)
                if mapsvg_obj == nil {
                    fmt.Fprintf(os.Stderr, "can't create SVG object from " + attrs.InputFile)
                    return
                }

                if len(attrs.InlineData) > 0 {
                    mapdata = attrs.InlineData
                } else if maptype == "counties" {
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
                        fmt.Fprintf(os.Stderr, "%s: %s\n", attrs.InputFile, errmsg)
                    }
                }

                ret := re_svgext.Find([]byte(attrs.OutputFile))
                if ret == nil {
                    // going to call ImageMagick's 'convert' because I can't find
                    // a damn SVG package that can write to a non-SVG image and I
                    // don't have the chops to write one.
                    imagemagick := config.General["imagemagick_convert"]
                    if len(imagemagick) == 0 {
                        imagemagick = "convert"
                    }
                    cmd := exec.Command(imagemagick, "svg:-", "-resize", attrs.OutputSize, "png:-")
                    convert_stdin, err := cmd.StdinPipe()
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "exec convert: %s\n", err.Error())
                        return
                    }
                    go func() {
                        defer convert_stdin.Close()
                        io.WriteString(convert_stdin, svg_coloured)
                    }()

                    // grab PNG data and cram it into an RGBA image
                    png_data, err := cmd.Output()
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "read from convert: %s\n", err.Error())
                        return
                    }
                    png_reader := s.NewReader(string(png_data))
                    img, _, err := image.Decode(png_reader)
                    b := img.Bounds()
                    img_rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
                    draw.Draw(img_rgba, img_rgba.Bounds(), img, b.Min, draw.Src)

                    ah_hates_legends(img_rgba, mincount, config.Colours, config.LADefaults, attrs)

                    annotate(img_rgba, config.LADefaults, attrs, mapdata)
                    outfile_handle, err := os.Create(attrs.OutputFile)
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "can't create '" + attrs.OutputFile + "': " + err.Error())
                        return
                    }
                    if err := png.Encode(outfile_handle, img_rgba); err != nil {
                        outfile_handle.Close()
                        log.Fatal(err)
                    }
                } else {
                    // just going back to an SVG file
                    err := ioutil.WriteFile(attrs.OutputFile, []byte(svg_coloured), 0666)
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "can't write to '" + attrs.OutputFile + "': " + err.Error())
                        return
                    }
                }

            }(attrs, maptype, data)
        }
    }

    wg.Wait()

}

// vim:ts=4:et:
// ex:ai:sw=4:ts=1000:
