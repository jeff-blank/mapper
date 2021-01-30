package main

import (
	"database/sql"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	re "regexp"
	"strconv"
	s "strings"
	"time"

	"github.com/golang/freetype"
	"github.com/jeff-blank/mapper/pkg/config"
	"github.com/jeff-blank/mapper/pkg/svgxml"
	log "github.com/sirupsen/logrus"
)

// suck in count data
func dbData(dbconfig map[string]string) (map[string]int, map[string]int) {

	state_counts := make(map[string]int)
	county_counts := make(map[string]int)

	dbh, err := sql.Open(dbconfig["type"],
		dbconfig["type"]+"://"+dbconfig["username"]+":"+
			dbconfig["password"]+"@"+dbconfig["host"]+"/"+
			dbconfig["name"]+dbconfig["connect_opts"])
	if err != nil {
		log.Fatal("sql.Open(): ", err)
	}

	query :=
		"select " +
			dbconfig["state_column"] + ", " +
			dbconfig["county_column"] + ", " +
			dbconfig["tally_column"] + " " +
			"from " +
			dbconfig["tables"] + " " +
			dbconfig["where"] + " " +
			dbconfig["group_by"]
	log.Debug(query)
	rows, err := dbh.Query(query)
	if err != nil {
		log.Fatal("dbh.Query(): ", err)
	}

	defer rows.Close()
	for rows.Next() {
		var state, county string
		var count int
		if err := rows.Scan(&state, &county, &count); err != nil {
			log.Fatal("rows.Scan(): ", err)
		}
		state_counts[state] += count
		stateCounty := s.Replace(state+" "+county, " ", "_", -1)
		county_counts[stateCounty] = count
	}
	if err := rows.Err(); err != nil {
		log.Fatal("rows.Err(): ", err)
	}

	return state_counts, county_counts

}

func colourSvgData(mapsvg_obj *svgxml.SVG, data map[string]int, re_fill *re.Regexp, colours map[string]string, mincount []int) (string, []string) {

	var errors []string

	for id, count := range data {
		for _, mc := range mincount {
			if count >= mc {
				element := svgxml.FindPathById(mapsvg_obj, id)
				if element != nil {
					element.Style = string(re_fill.ReplaceAll([]byte(element.Style), []byte("${1}"+colours[strconv.Itoa(mc)])))
				} else {
					errors = append(errors, "'"+id+"' not found")
				}
			}
		}
	}
	return string(svgxml.SVG2XML(mapsvg_obj, true)), errors
}

func annotate(img *image.RGBA, defaults config.LegendAnnotateParams, attrs config.MapSet, data map[string]int) {

	ann_x := defaults.AnnotationX
	ann_y := defaults.AnnotationY
	timefmt := defaults.AnnotationTimeFmt
	fontfile := defaults.AnnotationFontFile
	fontsize := defaults.AnnotationFontSize
	ann_str := defaults.AnnotationString

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
		log.Fatalf("annotate(): read font file '%s': %v", fontfile, err)
	}
	font, err := freetype.ParseFont(fontdata)
	if err != nil {
		log.Fatal("annotate(): ParseFont(): ", err)
	}

	fontCtx := freetype.NewContext()
	fontCtx.SetDPI(72.0)
	fontCtx.SetFont(font)
	fontCtx.SetFontSize(fontsize)
	fontCtx.SetClip(img.Bounds())
	fontCtx.SetDst(img)
	fontCtx.SetSrc(image.Black)
	pt := freetype.Pt(int(ann_x), int(ann_y)+int(fontCtx.PointToFixed(fontsize)>>6))

	total_hits := 0
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
		_, err = fontCtx.DrawString(line, pt)
		if err != nil {
			log.Fatal("annotate(): fontCtx.DrawString(): ", err)
		}
		pt.Y += fontCtx.PointToFixed(fontsize * 1.2)
	}
}

func ahHatesLegends(img *image.RGBA, mincount []int, colours map[string]string, defaults config.LegendAnnotateParams, attrs config.MapSet) {
	fontfile := defaults.LegendFontFile
	fontsize := defaults.LegendFontSize
	gravity := defaults.LegendGravity
	legendX := defaults.LegendX
	legendY := defaults.LegendX
	orient := defaults.LegendOrient
	cellW := defaults.LegendCellWidth
	cellH := defaults.LegendCellHeight
	cellGap := defaults.LegendCellGap

	if len(attrs.LegendAnnotate.LegendFontFile) > 0 {
		fontfile = attrs.LegendAnnotate.LegendFontFile
	}

	if attrs.LegendAnnotate.LegendFontSize > 0 {
		fontsize = attrs.LegendAnnotate.LegendFontSize
	}

	if len(attrs.LegendAnnotate.LegendGravity) > 0 {
		gravity = attrs.LegendAnnotate.LegendGravity
	}

	if len(attrs.LegendAnnotate.LegendGravity) > 0 {
		gravity = attrs.LegendAnnotate.LegendGravity
	}

	if gravity == "-" {
		legendX = attrs.LegendAnnotate.LegendX
		legendY = attrs.LegendAnnotate.LegendY
	}

	if len(attrs.LegendAnnotate.LegendOrient) > 0 {
		orient = attrs.LegendAnnotate.LegendOrient
	}

	if attrs.LegendAnnotate.LegendCellWidth > 0 {
		cellW = attrs.LegendAnnotate.LegendCellWidth
	}
	if attrs.LegendAnnotate.LegendCellHeight > 0 {
		cellH = attrs.LegendAnnotate.LegendCellHeight
	}
	if attrs.LegendAnnotate.LegendCellGap > 0 {
		cellGap = attrs.LegendAnnotate.LegendCellGap
	}

	fontdata, err := ioutil.ReadFile(fontfile)
	if err != nil {
		log.Fatalf("ahHatesLegends(): read font file '%s': %v", fontfile, err)
	}
	font, err := freetype.ParseFont(fontdata)
	if err != nil {
		log.Fatal("ahHatesLegends(): ParseFont(): ", err)
	}
	b := img.Bounds()
	fontCtx := freetype.NewContext()
	fontCtx.SetDPI(72.0)
	fontCtx.SetFont(font)
	fontCtx.SetFontSize(fontsize)
	fontCtx.SetClip(b)
	fontCtx.SetDst(img)
	fontCtx.SetSrc(image.Black)

	legendWidth := cellW
	legendHeight := cellH
	if orient == "vertical" {
		legendHeight = len(colours)*(cellH+cellGap) - cellGap
	} else {
		legendWidth = len(colours)*(cellW+cellGap) - cellGap
	}

	boxX := 0
	boxY := 0
	log.Debugf("gravity: %s; coords: %dx%d", gravity, legendX, legendY)
	if gravity == "-" {
		boxX = legendX
		boxY = legendY
	} else {
		if s.ToLower(gravity)[0] == 's' {
			boxY = b.Dy() - legendHeight
		}
		if s.ToLower(gravity)[1] == 'e' {
			boxX = b.Dx() - legendWidth
		}
	}

	for i, mc := range mincount {
		colRed, _ := strconv.ParseUint(colours[strconv.Itoa(mc)][0:2], 16, 8)
		colGreen, _ := strconv.ParseUint(colours[strconv.Itoa(mc)][2:4], 16, 8)
		colBlue, _ := strconv.ParseUint(colours[strconv.Itoa(mc)][4:6], 16, 8)
		fill := color.RGBA{uint8(colRed), uint8(colGreen), uint8(colBlue), 255}
		draw.Draw(img, image.Rect(boxX, boxY, boxX+cellW, boxY+cellH),
			&image.Uniform{fill}, image.ZP, draw.Src)
		if orient == "vertical" {
			boxY += cellH + cellGap
		} else {
			boxX += cellW + cellGap
		}

		label := strconv.Itoa(mc)
		if i == len(mincount)-1 {
			label = label + "+"
		} else if mincount[i+1] != (mc + 1) {
			label = label + "-" + strconv.Itoa(mincount[i+1]-1)
		}
		var textX, textY int
		if orient == "vertical" {
			textX = boxX + 4
			textY = boxY - cellH + int(fontCtx.PointToFixed(fontsize)>>6)
		} else {
			textX = boxX - cellW + 4
			textY = boxY + int(fontCtx.PointToFixed(fontsize)>>6)
		}
		fpt := freetype.Pt(textX, textY)
		_, err = fontCtx.DrawString(label, fpt)
		if err != nil {
			log.Fatal("ahHatesLegends(): fontCtx.DrawString(): ", err)
		}
	}
}

// Prune county data for *states* that don't appear in the given map. This is
// so that counties in states outside the map don't cause error messages and
// counties in the map that have a different (incorrect) name in the data do
// generate errors.
func pruneCounties(mapsvg_obj *svgxml.SVG, mapData, stateData map[string]int) map[string]int {

	var mapStateList []string

	countyData_new := make(map[string]int)

	// first, make a list of all states in the map using
	// stateData as the source of state names
	for _, g := range mapsvg_obj.G {
		for state := range stateData {
			if s.Index(g.Id, state+"_") == 0 {
				mapStateList = append(mapStateList, state+"_")
			}
		}
	}

	// next, search county names in data for states found in
	// the map and copy only county data entries for those
	// found states
	for stateCounty, sc_count := range mapData {
		found_state := false
		for _, state_ := range mapStateList {
			if s.Index(stateCounty, state_) == 0 {
				found_state = true
				break
			}
		}
		if found_state == true {
			countyData_new[stateCounty] = sc_count
		}
	}
	return countyData_new
}
