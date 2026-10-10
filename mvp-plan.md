# Goal

Deliver multi-user job-search service MVP: 

- secure user accounts and profile/resume intake

- per-user job matches and activity

- a durable daily job pipeline backed by SQLite

- job listings from rendered database entries

- usable admin/onboarding UI

# Implementation Summary

You are building a Golang based web server application, which will have additional services running alongside it for data processing. Each service stands alone, and you should seek to reduce coupling as much as possible - ideally, services only know about the shared database shape definitions, and nothing else about one another.

At a high level, the MVP contains these services:

- The web server: Has pages for user auth, onboarding flow, a top level list of daily job digests, individual job digest, and admin user management pages.

- Job Board Search: today, this just detects an empty DB table for the list of job sites, reads the appropriate JSON file, and populates the job board table.

- Job Description Search: This scrapes the job descriptions from the identified jobs boards, ideally identifying the date of posting, and puts them in the job description table.

- Job Match Search: This pulls the JDs from the database and determines which are compatible with a given candidate, and then writes the appropriate data to the job digest table.

- Shared SQLite database: holds all data including user auth, job data, user config, history, etc.

# Implementation Plan

1. **Basic Webserver** 
   You will implement a basic golang webserver, preferring the stdlib over outside libraries. It must support user authentication via both emailed one-time code and password, generating pages from server rendered templates, and will in the future support active javascript.
   Onboarding flow: signup with email plus one-time code or password; the email address must be confirmed before the account is marked live; then the user uploads the documents used for matching (a resume, and/or a doc of job experience as disconnected bullets), profile elements are assembled, and a user-specific matcher run kicks off across the previous week of data.

2. **Job Board Search**
   A separate executable that starts up once per day, runs the top level adzuna/serpapi/future data source searches, and scrapes those job entries to identify new job boards on existing or as yet unimplemented ATS systems. Make sure this work is designed with parallelism in mind: one worker per high-level source (one Adzuna worker, one SerpAPI worker), each with source-specific processing. Boards found on ATS systems that have no adapter yet are still recorded in the table with their ATS type and an unsupported flag until an adapter exists; they are never dropped. The final output is adding job boards to the table in sqlite, and then updating the external json file for them.
   Discovery upgrade path: scraping search entries only finds boards those marketplaces list. When yield plateaus, evaluate existing MIT-licensed harvesters (OpenRoles for Common Crawl/sitemap harvesting, JobHunter for directory/portfolio mining of new employers) instead of building a crawler. Either way the import boundary is a candidate export (provider, identifier, url, provenance, discovery timestamp, upstream status) feeding our own deduplication and board validation; discovered candidates are never auto-activated, upstream transient failures are not invalid boards, and boards with zero current jobs are not invalid.

3. **Job Description Search**
   Separate executable that starts after the board search is done for the day, scrapes all job boards for job listings, and inserts the job descriptions for any not previously found or updated JDs to insert into the sqlite table. This should have a way to identify different versions of the same JD as new entries in the table, without overwriting old ones.
   Versioning rule: a listing's identity is its canonical URL, never its title. Metadata columns update in place on every fetch; a new version row is inserted only when the content hash of (title + salary + normalized body text) differs from the latest version, where body text is HTML-stripped and whitespace-collapsed. This keeps repost timestamp refreshes and markup churn from creating versions. Every version row keeps its own found_at and full text; old versions are never overwritten or pruned.

4. **Job Match Search**
   Separate executable that starts after JD population is done for the day, runs for each user & generates matches. Must take some high level filters from the user config, run the appropriate matcher (support user level overrides to a global default matcher, which will at first be jev, but implementation needs to be classifier agnostic). Classifier failure should generally result in a retry, but the classifier handler should have graceful error handling, including exiting with a reported failure (which is neither a match nor non-match, simply categorize as a failure) and exponential backoff.

5. **Shared SQLite Database: clear setup script that creates tables matching the data shapes.**
   Basic shapes must include:
   1. Job Boards:
      board_id
      board_url
      company
   2. Job Descriptions
      job_description_id - unique ID that identifies this listing, probably the url string. doesn't need to include 
      board_id - the board it came from
      job_description_version - initial finds are always 1, add one and insert for each update version found over time
      datetime_found - for identifying freshness of jobs, this should be the time our process read the JD
      posted_at - the source-reported posting/update time, nullable (some sources do not provide it). Freshness cutoffs prefer posted_at and fall back to datetime_found
      content_hash - hash of (title + salary + normalized body text); a change from the latest version creates the next job_description_version
   3. Job matches
      job_match_id
      job_description_id
      job_match_rating - 1-5 scale
      job_match_data - this will be raw data from the matcher we use, should be like a json blob or similar field. Jev vs Haiku might fill it differently, so just needs to be open for now
   4. Users - obviously based on auth
   5. User config - holds the user's resume/job history information, job titles, search preferences, blocked companies, etc
   6. User history - login and config change history. Logins should include inbound IP information
   7. job_history - one shared table used by every process: service, started_at, finished_at, status (success/failure/partial), items_processed, and an error blob for surfacing to the admin. Processes report success/failure, total items processed, have somewhere to drop a blob of data for errors/specific logs to surface to the admin. This will feed back into the admin interface (separate page from user management)

   The table shapes live in a shared Go library that publishes a schema version number. At startup the orchestrator checks the database's current schema version and applies any necessary upgrades before services run. CI must verify — both enforced on GitHub and runnable locally — that the orchestrator can start on the previous schema version and upgrade to the new one.

# Acceptance Criteria
<!-- Checked items are verified in this repository (tests/gates passing locally as of the final integration review). Unchecked items require external credentials/production infrastructure and are listed in agent-mail/from-orchestrator/to-operator--report-final.md. -->

- [x] Fetches Greenhouse, Lever, and Ashby boards with full JD content; Workday summaries; Jobvite listings *(implemented, fixture-tested; no live provider calls made)*
- [x] Pulls jobs from Adzuna (≤100 calls/run, 8-day window), Climatebase (server-side date filter), and SerpAPI Google Jobs (3 queries × 2 pages = 6 credits max) *(limits enforced and tested; no live calls)*
- [x] HN "who's hiring" scraping with per-comment positive/negative cache and retry-on-failure
- [x] Appropriate retry and exponential backoff logic to avoid getting IP banned by anti-scraping features
- [x] Provider-neutral LLM classification
- [x] All pages beyond login require user auth
- [x] Admin user management: issue/revoke tokens, approve/suspend users
- [x] All services publish a healthcheck route (liveness only)
- [x] Container deploy: docker-compose behind an nginx reverse proxy *(compose config verified; live deploy at jobs.willwagner.me requires infrastructure)*
- [ ] GHCR releases gated on a `VERSION` bump per PR; tested-commit tagging *(CI configured for this; a real GHCR publish requires a runner run and registry access)*
- [ ] Mail integration with jobs@willwagner.me outbox *(SMTP wired and config-tested; sending requires live credentials)*
- [x] > 90% coverage per service, with 100% passing tests
