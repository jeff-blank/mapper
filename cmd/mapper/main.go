package main

import (
	"flag"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	re "regexp"
	"sort"
	"strconv"
	s "strings"
	"sync"

	"github.com/jeff-blank/mapper/pkg/config"
	"github.com/jeff-blank/mapper/pkg/svgxml"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// set up integer array sorting
type IntArray []int

func (list IntArray) Len() int           { return len(list) }
func (list IntArray) Swap(a, b int)      { list[a], list[b] = list[b], list[a] }
func (list IntArray) Less(a, b int) bool { return list[a] < list[b] }

func main() {

	var wg sync.WaitGroup

	configFile := flag.String("conf", "mapper.yml", "configuration file")
	logDebug := flag.Bool("d", false, "debug-level logging")
	flag.Parse()

	if *logDebug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	cfg := config.New(*configFile)

	// make sorted list of keys (minimum counts) for later comparisons
	mincount := make([]int, len(cfg.Colours))
	i := 0
	for k := range cfg.Colours {
		k_i, _ := strconv.ParseInt(k, 0, 64)
		mincount[i] = int(k_i)
		i++
	}

	sort.Sort(IntArray(mincount))

	re_fill, err := re.Compile(`(fill:#)......`)
	if err != nil {
		log.Fatal("re.Compile() fill: ", err)
	}

	re_svgext, err := re.Compile(`\.svg$`)
	if err != nil {
		log.Fatal("re.Compile() .svg: ", err)
	}

	state_data, county_data := dbData(cfg.DbParam)

	for maptype, mapset := range cfg.Maps {
		var data map[string]int

		if maptype == "states" {
			data = state_data
		} else {
			data = county_data
		}

		for _, attrs := range mapset {
			wg.Add(1)
			go func(attrs config.MapSet, maptype string, mapdata_default map[string]int) {

				var mapdata map[string]int

				if len(cfg.DbParam["where"]) > 0 && len(attrs.DbWhere) > 0 {
					newDbConfig := make(map[string]string)
					for k, v := range cfg.DbParam {
						log.Debugf("newDbConfig[%s] = %s", k, v)
						newDbConfig[k] = v
					}
					newDbConfig["where"] = cfg.DbParam["where"] + " and " + attrs.DbWhere
					state_new, county_new := dbData(newDbConfig)
					if maptype == "states" {
						mapdata = state_new
					} else {
						mapdata = county_new
					}
					log.Debug(mapdata)
				} else {
					mapdata = mapdata_default
				}

				defer wg.Done()
				//defer os.Stderr.Close()

				mapsvg, err := ioutil.ReadFile(attrs.InputFile)
				if err != nil {
					log.Error("can't read '" + attrs.InputFile + "': " + err.Error())
					return
				}

				mapsvg_obj := svgxml.XML2SVG(mapsvg)
				if mapsvg_obj == nil {
					log.Error("can't create SVG object from " + attrs.InputFile)
					return
				}

				if len(attrs.InlineData) > 0 {
					mapdata = attrs.InlineData
				}
				if maptype == "counties" {
					mapdata = pruneCounties(mapsvg_obj, mapdata, state_data)
				}

				errlist := colourSvgData(mapsvg_obj, mapdata, re_fill, cfg.Colours, mincount)
				if len(errlist) > 0 {
					for _, errmsg := range errlist {
						log.Warnf("%s: %s\n", attrs.InputFile, errmsg)
					}
				}

				ret := re_svgext.Find([]byte(attrs.OutputFile))
				if ret == nil {
					// going to call ImageMagick's 'convert' because I can't find
					// a damn SVG package that can write to a non-SVG image and I
					// don't have the chops to write one.
					imagemagick := cfg.General["imagemagick_convert"]
					if len(imagemagick) == 0 {
						imagemagick = "convert"
					}
					cmd := exec.Command(imagemagick, "svg:-", "-resize", attrs.OutputSize, "png:-")
					convert_stdin, err := cmd.StdinPipe()
					if err != nil {
						log.Error("exec convert: ", err)
						return
					}
					go func() {
						defer convert_stdin.Close()
						io.WriteString(convert_stdin, string(svgxml.SVG2XML(mapsvg_obj, false)))
					}()

					// grab PNG data and cram it into an RGBA image
					png_data, err := cmd.Output()
					if err != nil {
						log.Debugf("%s svg:- -resize %s png:-", imagemagick, attrs.OutputSize)
						log.Error("read from convert: ", err)
						return
					}
					png_reader := s.NewReader(string(png_data))
					img, _, err := image.Decode(png_reader)
					b := img.Bounds()
					imgRbga := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
					draw.Draw(imgRbga, imgRbga.Bounds(), img, b.Min, draw.Src)

					if len(cfg.LADefaults.LegendFontFile) > 0 || len(attrs.LegendAnnotate.LegendFontFile) > 0 {
						ahHatesLegends(imgRbga, mincount, cfg.Colours, cfg.LADefaults, attrs)
					}

					annotate(imgRbga, cfg.LADefaults, attrs, mapdata)
					outfile_handle, err := os.Create(attrs.OutputFile)
					if err != nil {
						log.Errorf("can't create '%s': %v", attrs.OutputFile, err)
						return
					}
					if err := png.Encode(outfile_handle, imgRbga); err != nil {
						outfile_handle.Close()
						log.Fatalf("close png file '%s': %v", attrs.OutputFile, err)
					}
				} else {
					ahHatesLegends(mapsvg_obj, mincount, cfg.Colours, cfg.LADefaults, attrs)
					annotate(mapsvg_obj, cfg.LADefaults, attrs, mapdata)
					svgText := svgxml.SVG2XML(mapsvg_obj, true)
					err := ioutil.WriteFile(attrs.OutputFile, svgText, 0666)
					if err != nil {
						log.Errorf("can't write to '%s': %v", attrs.OutputFile, err)
						return
					}
				}

			}(attrs, maptype, data)
		}
	}

	wg.Wait()
}
