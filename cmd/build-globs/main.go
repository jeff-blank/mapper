package main

import (
	"database/sql"
	"flag"
	"io/ioutil"
	s "strings"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type LegendAnnotateParams struct {
	LegendGravity      string  `yaml:"legend_gravity"`
	LegendOrient       string  `yaml:"legend_orient"`
	LegendFontFile     string  `yaml:"legend_fontfile"`
	LegendFontSize     float64 `yaml:"legend_fontsize"`
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
	SmallGlobSize map[string]int
	SmallGlobId   map[string]int
	LargeGlobSize map[string]int
	LargeGlobId   map[string]int
	NoGlobId      map[string]int
	NoGlobIdDb    int
}

type Residence map[string]int
type CountyList map[int]int
type Glob map[int]int
type Karta map[int]Glob

func findFreeGlobId(min, max int, ids map[int]int) int {
	for i := min; i <= max; i++ {
		if _, idInUse := ids[i]; !idInUse {
			return i
		}
	}
	return -1
}

func dbConnect(dbconfig map[string]string) *sql.DB {
	dbh, err := sql.Open(dbconfig["type"],
		dbconfig["type"]+"://"+dbconfig["username"]+":"+
			dbconfig["password"]+"@"+dbconfig["host"]+"/"+
			dbconfig["name"]+dbconfig["connect_opts"])
	if err != nil {
		log.Fatal("sql.Open(): ", err)
	}
	return dbh
}

func getResidences(dbh *sql.DB) Residence {
	// first one left blank (all residences when referenced later)
	results := make(Residence)
	rows, err := dbh.Query(`select label, home from residences order by label desc`)
	if err != nil {
		log.Fatal("getResidences(): dbh.Query(): ", err)
	}

	defer rows.Close()
	for rows.Next() {
		var (
			label    string
			countyId int
		)
		if err := rows.Scan(&label, &countyId); err != nil {
			log.Fatal("getResidences(): rows.Scan(): ", err)
		}
		results[label] = countyId
	}
	return results
}

// suck in count data
func dbData(dbh *sql.DB, queryExtra string) CountyList {

	results := make(CountyList)

	query := `select distinct
	cm.id
from
	hits h,
	bills b,
	counties_master cm
where
	(h.country='US' and
	h.state <> 'DC' and
	h.bill_id = b.id and
	h.county = cm.county and
	h.state = cm.state)` + queryExtra
	rows, err := dbh.Query(query)
	if err != nil {
		log.Fatal("dbData(): dbh.Query(): ", err)
	}

	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			log.Fatal("dbData(): rows.Scan(): ", err)
		}
		results[id] = 1
	}
	if err := rows.Err(); err != nil {
		log.Fatal("dbData(): rows.Err(): ", err)
	}

	return results
}

func findGlob(query *sql.Stmt, cid int, counties map[int]int, glob Glob) {
	log.Debugf("getting glob for %#v\n", cid)

	rows, err := query.Query(cid)
	if err != nil {
		log.Fatal("findGlob(): query.Query(): ", err)
	}
	defer rows.Close()

	if _, found := counties[cid]; found {
		log.Debugf("found %#v in counties list; deleting", cid)
		delete(counties, cid)
	}
	glob[cid] = 1
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			log.Fatal("findGlob(): rows.Scan(): ", err)
		}
		adjCounty := id
		log.Debugf("%#v -> %#v", cid, adjCounty)
		if _, found := counties[adjCounty]; found {
			log.Debugf("%v: hit found in %v, moving latter to glob", cid, adjCounty)
			findGlob(query, id, counties, glob)
			delete(counties, adjCounty)
			glob[adjCounty] = 1
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal("findGlob(): rows.Err(): ", err)
	}
}

func dbAddGlobs(dbh *sql.DB, karta Karta, residence, dbAction string, config Config) {
	var (
		err      error
		globStmt *sql.Stmt
	)

	if dbAction == "insert" {
		globStmt, err = dbh.Prepare(`insert into county_globs (county_id, glob_id) values ($1, $2)`)
	} else if dbAction == "update" {
		globStmt, err = dbh.Prepare(`update county_globs set ` + residence + `_glob_id = $2 where county_id = $1`)
	} else {
		log.Fatalf("dbAddGlobs(): invalid action '%s'", dbAction)
	}
	if err != nil {
		log.Fatal("dbAddGlobs(): prepare globStmt (%s/%s): %v", residence, dbAction, err)
	}
	if dbAction == "insert" {
		_, err = dbh.Exec(`delete from county_globs`)
		if err != nil {
			log.Fatal("dbAddGlobs(): delete all from county_globs: %v", err)
		}
	}
	for gid, g := range karta {
		if gid >= config.NoGlobId["min"] && gid <= config.NoGlobId["max"] {
			gid = config.NoGlobIdDb
		}
		for cid, _ := range g {
			res, err := globStmt.Exec(cid, gid)
			if err != nil {
				log.Fatalf("dbAddGlobs(): %s county %d with glob id %d: %v", dbAction, cid, gid, err)
			}
			ra, err := res.RowsAffected()
			if err != nil {
				log.Fatalf("dbAddGlobs(): %s county %d with glob id %d: can't get # rows affected: %v", dbAction, cid, gid, err)
			}
			if ra != 1 {
				//_ = dbh.QueryRow(`select * from county_globs where county_id = `+ strconv.Itoa(cid)).Scan(&county, &state)
				log.Fatalf("dbAddGlobs(): %s county %d with glob id %d (%s): %d rows affected", dbAction, cid, gid, residence, ra)
			}
		}
	}
}

func main() {
	var config Config

	config_file := flag.String("conf", "mapper.yml", "configuration file")
	logDebug := flag.Bool("d", false, "debug-level logging")
	flag.Parse()

	if *logDebug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	yamlcfg, err := ioutil.ReadFile(*config_file)
	if err != nil {
		log.Fatalf("read config file '%s': %v", *config_file, err)
	}

	err = yaml.Unmarshal(yamlcfg, &config)
	if err != nil {
		log.Fatal("yaml.Unmarshal(): ", err)
	}

	// TODO: add to config file
	config.NoGlobIdDb = 9999
	config.NoGlobId = map[string]int{"min": 9000, "max": 9999}
	config.SmallGlobSize = map[string]int{"min": 2, "max": 9}
	config.SmallGlobId = map[string]int{"min": 100, "max": 4999}
	config.LargeGlobSize = map[string]int{"min": 10, "max": 5000}
	config.LargeGlobId = map[string]int{"min": 2, "max": 9}

	dbh := dbConnect(config.DbParam)
	defer dbh.Close()

	klumpKartaSamling := make(map[string]Karta, 0)
	adjacencyQuery, err := dbh.Prepare(`select
	cm.id
from
	counties_master cm_in,
	counties_master cm,
	counties_graph cg
where
	cm_in.id = $1 and
	((cg.a=cm_in.id and cg.b=cm.id)
	 or (cg.b=cm_in.id and cg.a=cm.id))`)
	if err != nil {
		log.Fatalf("prepare adjacencyQuery: %v", err)
	}

	/*
		countyNameQuery, err := dbh.Prepare(`select county, state from counties_master where id=$1`)
		if err != nil {
			log.Fatalf("prepare countyNameQuery: %v", err)
		}
	*/

	residenceList := getResidences(dbh)
	for residence, home := range residenceList {
		var residenceWhere string
		log.Debug("residence: ", residence)
		if residence != "_all" {
			residenceWhere = `and b.residence = '` + residence + `'`
		}

		countyList := dbData(dbh, residenceWhere)
		log.Debugf("start: %d counties", len(countyList))
		log.Debugf("%#v\n==========", len(countyList))

		globIds := make(map[int]int)
		klumpKarta := make(Karta)
		for {
			log.Debugf(">>>>> %d counties", len(countyList))
			log.Debugf(">>>>> %#v", len(countyList))
			if len(countyList) == 0 {
				break
			}
			glob := make(Glob)
			for key, _ := range countyList {
				findGlob(adjacencyQuery, key, countyList, glob)
				log.Debug(glob)
				break
			}
			var globId int
			for cid, _ := range glob {
				if cid == home {
					globId = 1
				}
			}
			if globId != 1 {
				if len(glob) >= config.SmallGlobSize["min"] && len(glob) <= config.SmallGlobSize["max"] {
					globId = findFreeGlobId(config.SmallGlobId["min"], config.SmallGlobId["max"], globIds)
				} else if len(glob) >= config.LargeGlobSize["min"] && len(glob) <= config.LargeGlobSize["max"] {
					globId = findFreeGlobId(config.LargeGlobId["min"], config.LargeGlobId["max"], globIds)
				} else if len(glob) == 1 {
					globId = findFreeGlobId(config.NoGlobId["min"], config.NoGlobId["max"], globIds)
				}
				log.Debug("globId=", globId)
				globIds[globId] = 1
			}
			globIds[globId] = 1
			klumpKarta[globId] = glob
		}
		log.Debugf("end: %d counties", len(countyList))
		klumpKartaSamling[s.ToLower(residence)] = klumpKarta
		/*
			for gid, g := range klumpKarta {
				//if len(g) < 2 { continue }
				if gid >= config.NoGlobId["min"] && gid <= config.NoGlobId["max"] {
					gid = config.NoGlobIdDb
				}
				fmt.Printf("%d(%d) => {\n", gid, len(g))
				for cid, _ := range g {
					var county, state string
					_ = countyNameQuery.QueryRow(cid).Scan(&county, &state)
					fmt.Printf("\t%s, %s\n", county, state)
				}
				fmt.Println("}")
			}
		*/
	}

	dbAddGlobs(dbh, klumpKartaSamling["_all"], "_all", "insert", config)
	for residence, karta := range klumpKartaSamling {
		if residence == "_all" {
			continue
		}
		dbAddGlobs(dbh, karta, residence, "update", config)
	}

}

// vim:ts=4:et:
// ex:ai:sw=4:ts=1000:
