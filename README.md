# mapper

## Setup

After cloning the repo:

```bash
$ go get github.com/golang/freetype gopkg.in/yaml.v2 github.com/lib/pq
```
Replace `github.com/lib/pq` with the package for your preferred database driver
and update the `_ "github.com/lib/pq"` import line in `mapper.go`. Then

```bash
$ go build mapper.go
```

## Format of input SVG files

An SVG file of states without counties should contain colourable paths named
for the states. Any naming/abbreviating convention can be used as long as it
matches what is produced by the database query (see next section). The paths
are expected to be found in the following SVG structure:

```xml
<svg ...> <g ...> <path id="state" style="...fill:#XXXXXX" /> </g> </svg>
```

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
  * etc.
* Cass County, ND:
  * `ND_Cass`
  * `North_Dakota_Cass`

As you might guess by this point, the SVG files don't actually need to be
maps. But the path names in the "county" map(s) need(s) to be prefixed with
the path names in the "state" map(s) for the code to work as written.

## Configuration file

The default configuration filename is `mapper.yml` in the current directory and
can be overridden with the `-conf` command-line flag. See
[example-mapper.yml](example-mapper.yml).

### Image-generation parameters

* The `colours` section defines minimum values that correspond to colours. Any
  specified value for the key `0` will be ignored; the states or counties are
  expected to be pre-coloured with a default colour.

  ```yaml
  colours:
    1: "f0f098"
    2: "38e0ff"
    5: "d050c0"
  ```

  This defines colours for regions with values 1, 2-4, and 5-or-greater.

* `annotation_str` interpolations in `legend_annotations_defaults` and
  per-map definitions:
  * `%t%` is replaced with the tally for visible regions
  * `%c%` is replaced with the count of visible regions with (non-zero) data
  * `%T%` is replaced with the current date/time per the `annotation_timefmt`
    attribute
    * The `annotation_timefmt` may be confusing to those who have not worked
      with date/time formatting in go; see https://godoc.org/time#Time.Format
* The `legend_annotations_defaults` section defines default parameters for
  the legend (colour/number key) and textual annotations to be added to
  images.
* Map definitions:
  * `infile`, `outfile`, `outsize`, and (if applicable) `regions_adjust` and
    `inline_data` must be specified per-map. The remaining attributes may
    override or be inherited from the `legend_annotations_defaults` section.
  * If you want `%c%` to refer to a number of _states_ while excluding other
    regions, set `regions_adjust` to the number of those non-state regions (DC,
    PR, etc.) that are treated as states for mapping purposes and have data
  * `inline_data` is a simple `region: tally` dataset; whether it is used for a
    state or a county map, the keys should match the fillable regions' ids


### Database configuration

The first six parameters in the example config map into the call to
`sql.Open()` as follows:

```go
//                   type       type       username    password  host      name   connect_opts
dbh, err := sql.Open(postgres, "postgres://db_username:db_passwd@db_server/db_name?sslmode=require")
```

The idea for the `schema` section is to get rows of the format
`state_name | county_name | county_tally`.  The `db_schema` in the example
config results in a query that looks like this:

```sql
select state, county, count(county) from events
  where country = 'US'
  group by state, county
```

The `where` and `group_by` attributes are not required in the file if not
required by your query.

Untested, but multiple-table access should work like this:

```yaml
database:
# [...]
state_column:   "s.abbr",
county_column:  "c.name",
tally_column:   "count(c.name) as count",
tables:         "state s, county c, events e",
where:          "where e.county_id = c.id and c.state_id = s.id",
group_by:       "group by s.abbr, c.name"
```

Leading to...

```sql
select s.abbr, c.name, count(c.name) from state s, county c, events e
  where e.county_id = c.id and c.state_id = s.id
  group by s.abbr, c.name
```

...which may or may not work&mdash;I'm just spitballin' here.

Also untested, but `tally_column` should work as a regular column containing
a number, provided each state/county combination occurs only once. In this
case, `group_by` would presumably be omitted.
