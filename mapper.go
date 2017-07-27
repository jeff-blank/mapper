package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "regexp"
    "strconv"
    "os"
    "jfb/svgxml"
)

type Config struct {
    // member  type        json element
    Colours map[string]string `json:"colours"`
}

func main() {

    var config Config
    var colors map[int]string

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

    // convert string-based JSON to all-integer map
    colors = make(map[int]string)
    for k, v := range config.Colours {
        k_i, _ := strconv.ParseInt(k, 0, 64)
        colors[int(k_i)] = v
    }

    mapsvg_obj := svgxml.XML2SVG(mapsvg)
    if mapsvg_obj == nil {
        os.Exit(1)
    }

    re_fill, err := regexp.Compile(`(fill:#)......`)
    if err != nil {
        panic(err)
    }

    element := svgxml.FindPathById(mapsvg_obj, "HI")

    if element != nil {
        element.Style = string(re_fill.ReplaceAll([]byte(element.Style), []byte("${1}ffaaaa")))
    } else {
        fmt.Println("'HI' not found")
    }

    // fmt.Println(string(svgxml.SVG2XML(mapsvg_obj, true)))

}

// vim:ai:sw=4:ts=8:
