package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "regexp"
    "strconv"
    "os"
    "sort"
    "jfb/svgxml"
)

type Config struct {
    Colours map[string]string `json:"colours"`
}

// set up integer array sorting

type IntArray []int

func (list IntArray) Len() int {
    return len(list)
}

func (list IntArray) Swap(a, b int) {
    list[a], list[b] = list[b], list[a]
}

func (list IntArray) Less(a, b int) bool {
    return list[a] < list[b]
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
        fmt.Fprintf(os.Stderr, "Unmarshal: ")
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

    data_in := map[string]int{ "HI": 1, "MN": 19, "MI": 20 }

    mapsvg_obj := svgxml.XML2SVG(mapsvg)
    if mapsvg_obj == nil {
        os.Exit(1)
    }

    re_fill, err := regexp.Compile(`(fill:#)......`)
    if err != nil {
        panic(err)
    }

    for state, state_count := range data_in {
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
