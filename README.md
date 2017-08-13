# mapper

## Format of input SVG files

An SVG file of states without counties should contain colourable paths named
for the states. Any naming/abbreviating convention can be used as long as it
matches what is produced by the database query (see next section). The paths
are expected to be found in the following SVG structure:

`<svg ...>
  <g ...>
    <path id="state" style="...fill:#XXXXXX" />
  </g>
</svg>`

An SVG file of counties should be structured the same way, except that the
path ids should be `"state_county_name"`&mdash;in other words, the state (as
found in the database), an underscore, and the county name as found in
the database. Due to the SVG files I use, there is code in `mapper` to
transform all spaces to underscores before searching the SVG file for
each element. Examples (as found in the file):

* Hennepin County, MN:
  * `MN_Hennepin`
  * `Minnesota_Hennepin`
* Saint Louis, MO (independent city, not county)
  * `MO_Saint_Louis_City`
  * `Missouri_St_Louis`
  * etc
* Cass County, ND:
  * `ND_Cass`
  * `North_Dakota_Cass`

As you might guess by this point, the SVG files don't actually need to be
maps. But the path names in the "county" map(s) need(s) to be prefixed with
the path names in the "state" map(s) for the code to work as written.

## Database configuration

The database configuration should be in `dbconfig.json`, which is
provided as [example-dbconfig.json](example-dbconfig.json).

```
{
  "db_server":{
    "dbtype":     "postgres",
    "dbname":     "your database name",
    "dbhost":     "dbserver.example.com",
    "dbopts":     "?sslmode=require"
  },
  "db_creds":{
    "username":   "your app's username",
    "password":   "username's password"
  },
  "db_schema":{
    "state_column":     "state",
    "county_column":    "county",
    "tally_column":     "count(county) as count",
    "tables":           "events",
    "where":            "",
    "group_by":         "group by state, county"
  }
}
```

`db_server` and `db_creds` map into the call to `sql.Open()` as follows:

```
... sql.Open(dbtype, "dbtype://username:password@dbhost/dbname?sslmode=require")
```

The idea for the `db_schema` section is to get rows of the format
`state_name | county_name | county_tally`.  The above `db_schema` results in
a query that looks like this:

```
select state, county, count(county) from events group by state, county
```

Untested, but multiple-table access should work like this:

```
  "db_schema":{
    "state_column":     "s.abbr",
    "county_column":    "c.name",
    "tally_column":     "count(c.name) as count",
    "tables":           "state s, county c",
    "where":            "where c.state_id = s.id",
    "group_by":         "group by s.abbr, c.name"
  }
```

```
select s.abbr, c.name, count(c.name) from events group by s.abbr, c.name
```

Also untested, but `tally_column` should work as a regular column containing
a number, provided each state/county combination occurs only once. In this
case, `group_by` would presumably be an empty string.
