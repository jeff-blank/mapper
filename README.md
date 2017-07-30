# mapper

Format of `dbconfig.json`:

```
{
  "db_server":{
    "dbtype":     "postgres",
    "dbname":     "your database name",
    "dbhost":     "dbserver.example.com"
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

In any case, the `db_data()` function counts by the number of rows containing
each county. There is currently no facility for keeping counts in another
column.
