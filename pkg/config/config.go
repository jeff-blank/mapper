package config

import (
	"io/ioutil"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type LegendAnnotateParams struct {
	LegendGravity      string  `yaml:"legend_gravity"`
	LegendX            []int   `yaml:"legend_x"`
	LegendY            []int   `yaml:"legend_y"`
	LegendOrient       string  `yaml:"legend_orient"`
	LegendFontFile     string  `yaml:"legend_fontfile"`
	LegendFontSize     float64 `yaml:"legend_fontsize"`
	LegendTextXOffset  []int   `yaml:"legend_text_x_offset"`
	LegendTextYOffset  []int   `yaml:"legend_text_y_offset"`
	LegendTextStyle    string  `yaml:"legend_text_style"`
	LegendCellWidth    int     `yaml:"legend_cell_width"`
	LegendCellHeight   int     `yaml:"legend_cell_height"`
	LegendCellGap      int     `yaml:"legend_cell_gap"`
	AnnotationFontFile string  `yaml:"annotation_fontfile"`
	AnnotationFontSize float64 `yaml:"annotation_fontsize"`
	AnnotationTimeFmt  string  `yaml:"annotation_timefmt"`
	AnnotationString   string  `yaml:"annotation_str"`
	AnnotationX        int     `yaml:"annotation_x"`
	AnnotationY        int     `yaml:"annotation_y"`
}

type MapSet struct {
	InputFile        string               `yaml:"infile"`
	OutputFile       string               `yaml:"outfile"`
	OutputSize       string               `yaml:"outsize"`
	RegionAdjustment int                  `yaml:"regions_adjust"`
	LegendAnnotate   LegendAnnotateParams `yaml:",inline"`
	InlineData       map[string]int       `yaml:"inline_data"`
	DbWhere          string               `yaml:"db_where"`
}

type Config struct {
	General       map[string]string
	Colours       map[string]string
	LADefaults    LegendAnnotateParams `yaml:"legend_annotation_defaults"`
	Maps          map[string][]MapSet  `yaml:"maps"`
	DbParam       map[string]string    `yaml:"database"`
	SmallGlobId   map[string]int       `yaml:"small_glob_id"`
	SmallGlobSize map[string]int       `yaml:"small_glob_size"`
	LargeGlobId   map[string]int       `yaml:"large_glob_id"`
	LargeGlobSize map[string]int       `yaml:"large_glob_size"`
	NoGlobIdDb    int                  `yaml:"no_glob_id_db"`
	NoGlobId      map[string]int       `yaml:"no_glob_id"`
}

func New(configFile string) *Config {
	config := &Config{}

	yamlcfg, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("read config file '%s': %v", configFile, err)
	}

	if err := yaml.Unmarshal(yamlcfg, &config); err != nil {
		log.Fatal("yaml.Unmarshal(): ", err)
	}

	return config
}
