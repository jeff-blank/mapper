package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io/ioutil"
    // s "strings"
    "os"
    "syscall"
)

type Config struct {
    // member  type        json element
    Colours map[string][]string `json:"colours"`
}

func main() {

    var config Config

    force := *(flag.Bool("f", false, "force map generation"))
    flag.Parse()

    if force {}

    source, err := ioutil.ReadFile("config.json")
    if err != nil {
	panic(err)
    }

    err = json.Unmarshal(source, &config)
    if err != nil {
	fmt.Fprintf(os.Stderr, "Unmarshal: ")
	panic(err)
    }

    //fmt.Println(config.Colours)

    syscall.Umask(022)
}

// vim:ai:sw=4:ts=8:
