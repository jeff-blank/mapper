package main

import (
	"database/sql"
	"flag"
	s "strings"

	"github.com/jeff-blank/mapper/pkg/config"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

const (
	ADJACENCY_SQL = `
		select
			cm.id
		from
			counties_master cm_in,
			counties_master cm,
			counties_graph cg
		where
			cm_in.id = $1 and
			((cg.a=cm_in.id and cg.b=cm.id)
			 or (cg.b=cm_in.id and cg.a=cm.id))`
)

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
	h.bill_id = b.id and
	h.county_id = cm.id)` + queryExtra
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

func dbAddGlobs(dbh *sql.DB, karta Karta, residence, dbAction string, cfg *config.Config) {
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
		if gid >= cfg.NoGlobId["min"] && gid <= cfg.NoGlobId["max"] {
			gid = cfg.NoGlobIdDb
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
				log.Fatalf("dbAddGlobs(): %s county %d with glob id %d (%s): %d rows affected", dbAction, cid, gid, residence, ra)
			}
		}
	}
}

func main() {
	configFile := flag.String("conf", "mapper.yml", "configuration file")
	logDebug := flag.Bool("d", false, "debug-level logging")
	flag.Parse()

	if *logDebug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	cfg := config.New(*configFile)

	log.Debugf("%#v", cfg)

	dbh := dbConnect(cfg.DbParam)
	defer dbh.Close()

	klumpKartaSamling := make(map[string]Karta, 0)
	adjacencyQuery, err := dbh.Prepare(ADJACENCY_SQL)
	if err != nil {
		log.Fatalf("prepare adjacencyQuery: %v", err)
	}

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
				if len(glob) >= cfg.SmallGlobSize["min"] && len(glob) <= cfg.SmallGlobSize["max"] {
					globId = findFreeGlobId(cfg.SmallGlobId["min"], cfg.SmallGlobId["max"], globIds)
				} else if len(glob) >= cfg.LargeGlobSize["min"] && len(glob) <= cfg.LargeGlobSize["max"] {
					globId = findFreeGlobId(cfg.LargeGlobId["min"], cfg.LargeGlobId["max"], globIds)
				} else if len(glob) == 1 {
					globId = findFreeGlobId(cfg.NoGlobId["min"], cfg.NoGlobId["max"], globIds)
				}
				log.Debug("globId=", globId)
				globIds[globId] = 1
			}
			globIds[globId] = 1
			klumpKarta[globId] = glob
		}
		log.Debugf("end: %d counties", len(countyList))
		klumpKartaSamling[s.ToLower(residence)] = klumpKarta
	}

	dbAddGlobs(dbh, klumpKartaSamling["_all"], "_all", "insert", cfg)
	for residence, karta := range klumpKartaSamling {
		if residence == "_all" {
			continue
		}
		dbAddGlobs(dbh, karta, residence, "update", cfg)
	}

}
