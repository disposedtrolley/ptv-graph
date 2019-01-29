# ptv-graph

`ptv-graph` will hopefully become an unofficial graph API into [PTV](https://www.ptv.vic.gov.au/)'s public transit timetable information. The service will ingest the [GTFS data](https://data.gov.au/dataset/ds-vic-ad4a0f7f-3e18-47d7-871d-7e19ae64648b/details) published by PTV into [neo4j](https://neo4j.com/); a graph database, and expose the data via a GraphQL API.

Currently, the scope of `ptv-graph` will be to ingest and expose static data supplied in PTV's GTFS files, however in the future these data may be enriched through the use of the [PTV Timetable API](https://www.data.vic.gov.au/data/dataset/ptv-timetable-api), for example to provide real-time arrival and disruption info.

This API is designed to be self-hosted rather than be provided as-a-service. Each release will include:

1. A docker-compose configuration to spin up the neo4j backend and GraphQL interface.
2. A toolset to retrieve GTFS data as supplied by PTV and ingestion into neo4j.

## Updating GTFS data from PTV

Use the `update-gtfs-data` binary in the `tools` directory to check for updated GTFS data from PTV and ingest them into neo4j. On first run, it will perform an initial load of the GTFS data. During subsequent runs it will check for updated data and if available, you will be prompted to continue the download and ingestion process.

For instance:

```
> ./tools/update-gtfs-data
Checking for new GTFS data from PTV...Done!
New GTFS data found. Update (Y/n)?

> Y
Downloading...Done!
Resetting neo4j...Done!
Ingesting to neo4j...Done!

Finished. GTFS data is now up-to-date as of 28/01/2019.
```
