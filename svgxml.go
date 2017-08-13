package svgxml

import (
    "encoding/xml"
    "fmt"
    "os"
    s "strings"
)

type PathDef struct {
    D       string  `xml:"d,attr"`
    Id      string  `xml:"id,attr"`
    Style   string  `xml:"style,attr"`
}

type GroupDef struct {
    Path    []PathDef   `xml:"path"`
    Id      string      `xml:"id,attr"`
    Xform   string      `xml:"transform,attr"`
    Style   string      `xml:"style,attr"`
}

type DefsDef struct {
    Id  string  `xml:"id,attr"`
}

type TSpanDef struct {
    Id      string  `xml:"id,attr"`
    X       string  `xml:"x,attr"`
    Y       string  `xml:"y,attr"`
    Label   string  `xml:",chardata"`
}

type TextDef struct {
    Style   string      `xml:"style,attr"`
    X       string      `xml:"x,attr"`
    Y       string      `xml:"y,attr"`
    Id      string      `xml:"id,attr"`
    TSpan   TSpanDef    `xml:"tspan"`
}

type SVG struct {
    XMLName xml.Name    `xml:"svg"`
    XMLNS   string      `xml:"xmlns,attr"`
    Width   string      `xml:"width,attr"`
    Height  string      `xml:"height,attr"`
    Id      string      `xml:"id,attr"`
    G       []GroupDef  `xml:"g"`
    Path    []PathDef   `xml:"path"`
    Defs    DefsDef     `xml:"defs"`
    Version string      `xml:"version,attr"`
    Text    []TextDef   `xml:"text"`
}

func XML2SVG(svg_xml []byte)(*SVG) {

    svg_obj := SVG{}
    err := xml.Unmarshal([]byte(svg_xml), &svg_obj)
    if err == nil {
        return &svg_obj
    } else {
        fmt.Fprintf(os.Stderr, "xml.Unmarshal error: %v\n", err)
        return nil
    }
}

func SVG2XML(imgxml *SVG, multi_line bool)([]byte) {

    var xml_txt []byte
    var err     error

    if multi_line {
        xml_txt, err = xml.MarshalIndent(imgxml, "", "    ")
    } else {
        xml_txt, err = xml.Marshal(imgxml)
    }
    if err != nil {
        fmt.Fprintf(os.Stderr, "xml.Marshal error: %v\n", err)
        return nil
    }

    svgtag_end := s.Index(string(xml_txt), "<svg") + 4
    xmlout := []byte(
        xml.Header +
        string(xml_txt[:svgtag_end]) +
        ` xmlns:svg="http://www.w3.org/2000/svg" xml:space="preserve"` +
        s.Replace(
            s.Replace(
                string(xml_txt[svgtag_end:]),
                "></path", " /",
                -1),
            "></defs", " /",
            -1))
    return xmlout
}

func FindPathById(mapsvg_obj *SVG, id string)(*PathDef) {
    for g_ind, group := range mapsvg_obj.G {
        for p_ind, path := range group.Path {
            if path.Id == id {
                return &(mapsvg_obj.G[g_ind].Path[p_ind])
            }
        }
    }
    return nil
}
