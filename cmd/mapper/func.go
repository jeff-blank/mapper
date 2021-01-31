package main

import (
	"database/sql"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	"reflect"
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

func colourSvgData(mapsvg_obj *svgxml.SVG, data map[string]int, re_fill *re.Regexp, colours map[string]string, mincount []int) []string {

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
	return errors
}

//func annotate(img *image.RGBA, defaults config.LegendAnnotateParams, attrs config.MapSet, data map[string]int)
func annotate(img interface{}, defaults config.LegendAnnotateParams, attrs config.MapSet, data map[string]int) {
	var (
		imgRgba     *image.RGBA
		imgSvg      *svgxml.SVG
		imgTypeStr  string
		lineSpacing int
	)

	imgType := reflect.TypeOf(img)
	if imgType == reflect.TypeOf(&image.RGBA{}) {
		imgTypeStr = "rgb"
		imgRgba = img.(*image.RGBA)
	} else if imgType == reflect.TypeOf(&svgxml.SVG{}) {
		imgTypeStr = "svg"
		imgSvg = img.(*svgxml.SVG)
	} else {
		log.Errorf("unknown image type '%s'", imgType.String)
		return
	}

	annX := defaults.AnnotationX
	annY := defaults.AnnotationY
	timefmt := defaults.AnnotationTimeFmt
	fontfile := defaults.AnnotationFontFile
	fontSize := defaults.AnnotationFontSize
	ann_str := defaults.AnnotationString
	textStyle := defaults.AnnotationTextStyle

	if len(defaults.AnnotationSpacing) > 0 {
		lineSpacing = defaults.AnnotationSpacing[0]
	}
	if attrs.LegendAnnotate.AnnotationX > 0 {
		annX = attrs.LegendAnnotate.AnnotationX
	}
	if attrs.LegendAnnotate.AnnotationY > 0 {
		annY = attrs.LegendAnnotate.AnnotationY
	}
	if len(attrs.LegendAnnotate.AnnotationFontFile) > 0 {
		fontfile = attrs.LegendAnnotate.AnnotationFontFile
	}
	if attrs.LegendAnnotate.AnnotationFontSize > 0 {
		fontSize = attrs.LegendAnnotate.AnnotationFontSize
	}
	if len(attrs.LegendAnnotate.AnnotationTimeFmt) > 0 {
		timefmt = attrs.LegendAnnotate.AnnotationTimeFmt
	}
	if len(attrs.LegendAnnotate.AnnotationString) > 0 {
		ann_str = attrs.LegendAnnotate.AnnotationString
	}
	if len(attrs.LegendAnnotate.AnnotationSpacing) > 0 {
		lineSpacing = attrs.LegendAnnotate.AnnotationSpacing[0]
	}

	if len(attrs.LegendAnnotate.AnnotationTextStyle) > 0 {
		textStyle = attrs.LegendAnnotate.AnnotationTextStyle
	}
	if len(textStyle) > 0 {
		textStyle += ";"
	}
	textStyle += "font-size:" + strconv.Itoa(int(fontSize)) + "px"

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
	annLines := s.Split(annotation, "\n")

	if imgTypeStr == "rgb" {
		fontdata, err := ioutil.ReadFile(fontfile)
		if err != nil {
			log.Errorf("annotate(): read font file '%s': %v", fontfile, err)
			return
		}
		font, err := freetype.ParseFont(fontdata)
		if err != nil {
			log.Error("annotate(): ParseFont(): ", err)
			return
		}

		fontCtx := freetype.NewContext()
		fontCtx.SetDPI(72.0)
		fontCtx.SetFont(font)
		fontCtx.SetFontSize(fontSize)
		fontCtx.SetClip(imgRgba.Bounds())
		fontCtx.SetDst(imgRgba)
		fontCtx.SetSrc(image.Black)
		pt := freetype.Pt(int(annX), int(annY)+int(fontCtx.PointToFixed(fontSize)>>6))

		for _, line := range annLines {
			_, err = fontCtx.DrawString(line, pt)
			if err != nil {
				log.Fatal("annotate(): fontCtx.DrawString(): ", err)
			}
			pt.Y += fontCtx.PointToFixed(fontSize * 1.2)
		}
	} else if imgTypeStr == "svg" {
		for i, line := range annLines {
			annotationDef := svgxml.TextDef{
				Id:    "Annotation",
				Style: textStyle,
				X:     strconv.Itoa(annX),
				Y:     strconv.Itoa(annY + int(float64(i)*(fontSize+float64(lineSpacing)))),
				TSpan: svgxml.TSpanDef{
					Id:    "AnnotationSpan",
					X:     strconv.Itoa(annX),
					Y:     strconv.Itoa(annY + int(float64(i)*(fontSize+float64(lineSpacing)))),
					Label: line,
				},
			}
			if len(imgSvg.Text) == 0 {
				imgSvg.Text = make([]svgxml.TextDef, 0)
			}
			imgSvg.Text = append(imgSvg.Text, annotationDef)
		}
	}

	log.Debugf("annotate(): done with %s image", imgTypeStr)
}

func svgLegend(img *svgxml.SVG, mincount []int, colours map[string]string, defaults config.LegendAnnotateParams, attrs config.MapSet) {
	var (
		textXOffset int
		textYOffset int
	)
	if len(img.Text) == 0 {
		img.Text = make([]svgxml.TextDef, 0)
	}

	legendX := -1
	legendY := -1
	gravity := defaults.LegendGravity
	orient := defaults.LegendOrient
	cellW := defaults.LegendCellWidth
	cellH := defaults.LegendCellHeight
	cellGap := defaults.LegendCellGap
	if len(defaults.LegendTextXOffset) > 0 {
		textXOffset = defaults.LegendTextXOffset[0]
	}
	if len(defaults.LegendTextXOffset) > 0 {
		textYOffset = defaults.LegendTextYOffset[0]
	}
	textSizePx := defaults.LegendFontSize
	legendTextStyle := defaults.LegendTextStyle

	if len(defaults.LegendX) > 0 {
		legendX = defaults.LegendX[0]
	}
	if len(defaults.LegendY) > 0 {
		legendY = defaults.LegendY[0]
	}
	if len(attrs.LegendAnnotate.LegendGravity) > 0 {
		gravity = attrs.LegendAnnotate.LegendGravity
	}
	if len(attrs.LegendAnnotate.LegendGravity) > 0 {
		gravity = attrs.LegendAnnotate.LegendGravity
	}
	if gravity == "-" {
		if len(attrs.LegendAnnotate.LegendX) > 0 {
			legendX = attrs.LegendAnnotate.LegendX[0]
		} else {
			return
		}
		if len(attrs.LegendAnnotate.LegendY) > 0 {
			legendY = attrs.LegendAnnotate.LegendY[0]
		} else {
			return
		}
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
	if len(attrs.LegendAnnotate.LegendOrient) > 0 {
		orient = attrs.LegendAnnotate.LegendOrient
	}
	if len(attrs.LegendAnnotate.LegendTextXOffset) > 0 {
		textXOffset = attrs.LegendAnnotate.LegendTextXOffset[0]
	}
	if len(attrs.LegendAnnotate.LegendTextXOffset) > 0 {
		textYOffset = attrs.LegendAnnotate.LegendTextYOffset[0]
	}
	if attrs.LegendAnnotate.LegendFontSize > 0 {
		textSizePx = attrs.LegendAnnotate.LegendFontSize
	}
	if len(attrs.LegendAnnotate.LegendTextStyle) > 0 {
		legendTextStyle = attrs.LegendAnnotate.LegendTextStyle
	}
	if len(legendTextStyle) > 0 {
		legendTextStyle += ";"
	}
	legendTextStyle += "font-size:" + strconv.Itoa(int(textSizePx)) + "px"

	rects := make([]svgxml.RectDef, 0)
	for i, mc := range mincount {
		var (
			xCoord int
			yCoord int
		)
		if orient == "vertical" {
			xCoord = legendX
			yCoord = legendY + i*(cellH+cellGap)
		} else {
			xCoord = legendX + i*(cellW+cellGap)
			yCoord = legendY
		}
		newRect := svgxml.RectDef{
			Id:     "Legend" + strconv.Itoa(i),
			Style:  "fill:#" + colours[strconv.Itoa(mc)],
			X:      strconv.Itoa(xCoord),
			Width:  strconv.Itoa(cellW),
			Y:      strconv.Itoa(yCoord),
			Height: strconv.Itoa(cellH),
		}
		rects = append(rects, newRect)

		label := strconv.Itoa(mc)
		if i == len(mincount)-1 {
			label = label + "+"
		} else if mincount[i+1] != (mc + 1) {
			label = label + "-" + strconv.Itoa(mincount[i+1]-1)
		}
		newText := svgxml.TextDef{
			Id:    "LegendText" + strconv.Itoa(i),
			X:     strconv.Itoa(xCoord + textXOffset),
			Y:     strconv.Itoa(yCoord + int(textSizePx) + textYOffset),
			Style: legendTextStyle,
			TSpan: svgxml.TSpanDef{
				Id:    "LegendSpan" + strconv.Itoa(i),
				Label: label,
				X:     strconv.Itoa(xCoord + textXOffset),
				Y:     strconv.Itoa(yCoord + int(textSizePx) + textYOffset),
			},
		}
		img.Text = append(img.Text, newText)
	}
	if len(img.G) == 0 {
		img.G = make([]svgxml.GroupDef, 0)
	}
	img.G = append(img.G, svgxml.GroupDef{Rect: rects})

}

func ahHatesLegends(img *image.RGBA, mincount []int, colours map[string]string, defaults config.LegendAnnotateParams, attrs config.MapSet) {
	fontfile := defaults.LegendFontFile
	fontsize := defaults.LegendFontSize
	gravity := defaults.LegendGravity
	orient := defaults.LegendOrient
	cellW := defaults.LegendCellWidth
	cellH := defaults.LegendCellHeight
	cellGap := defaults.LegendCellGap
	legendX := -1
	legendY := -1

	if len(defaults.LegendX) > 0 {
		legendX = defaults.LegendX[0]
	}
	if len(defaults.LegendY) > 0 {
		legendY = defaults.LegendY[0]
	}

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
		if len(attrs.LegendAnnotate.LegendX) > 0 {
			legendX = attrs.LegendAnnotate.LegendX[0]
		}
		if len(attrs.LegendAnnotate.LegendY) > 0 {
			legendY = attrs.LegendAnnotate.LegendY[0]
		}
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
