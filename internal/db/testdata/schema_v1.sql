CREATE TABLE schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE users (
	user_id INTEGER PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT,
	email_confirmed_at TEXT,
	status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','live','suspended')),
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE user_configs (
	user_id INTEGER PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
	resume_text TEXT,
	experience_text TEXT,
	desired_titles_json TEXT NOT NULL DEFAULT '[]',
	search_preferences_json TEXT NOT NULL DEFAULT '{}',
	blocked_companies_json TEXT NOT NULL DEFAULT '[]',
	matcher_name TEXT,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE user_history (
	user_history_id INTEGER PRIMARY KEY,
	user_id INTEGER REFERENCES users(user_id) ON DELETE SET NULL,
	event_type TEXT NOT NULL,
	inbound_ip TEXT,
	data_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE job_boards (
	board_id INTEGER PRIMARY KEY,
	board_url TEXT NOT NULL UNIQUE,
	company TEXT NOT NULL,
	ats_type TEXT,
	status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','unsupported')),
	metadata_json TEXT NOT NULL DEFAULT '{}',
	discovered_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE job_listings (
	job_listing_id INTEGER PRIMARY KEY,
	canonical_url TEXT NOT NULL UNIQUE,
	board_id INTEGER NOT NULL REFERENCES job_boards(board_id) ON DELETE RESTRICT,
	current_version INTEGER NOT NULL DEFAULT 0,
	last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	posted_at TEXT,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE job_listing_versions (
	job_listing_id INTEGER NOT NULL REFERENCES job_listings(job_listing_id) ON DELETE CASCADE,
	version INTEGER NOT NULL,
	found_at TEXT NOT NULL,
	posted_at TEXT,
	title TEXT NOT NULL,
	salary TEXT NOT NULL DEFAULT '',
	body_text TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	PRIMARY KEY (job_listing_id, version),
	UNIQUE (job_listing_id, content_hash)
);
CREATE TABLE job_matches (
	job_match_id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
	job_listing_id INTEGER NOT NULL REFERENCES job_listings(job_listing_id) ON DELETE CASCADE,
	job_listing_version INTEGER NOT NULL,
	rating INTEGER CHECK (rating BETWEEN 1 AND 5),
	decision TEXT NOT NULL DEFAULT 'deferred' CHECK (decision IN ('match','no_match','failure','deferred')),
	matcher_name TEXT NOT NULL,
	matcher_metadata_json TEXT NOT NULL DEFAULT '{}',
	job_match_data_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
	UNIQUE (user_id, job_listing_id, job_listing_version, matcher_name),
	FOREIGN KEY (job_listing_id, job_listing_version) REFERENCES job_listing_versions(job_listing_id, version)
);
CREATE TABLE job_history (
	job_history_id INTEGER PRIMARY KEY,
	service TEXT NOT NULL,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	status TEXT NOT NULL CHECK (status IN ('success','failure','partial')),
	items_processed INTEGER NOT NULL DEFAULT 0,
	data_json TEXT NOT NULL DEFAULT '{}',
	error_json TEXT,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO schema_migrations(version) VALUES (1);
INSERT INTO job_boards(board_id, board_url, company, ats_type, status) VALUES (1, 'https://example.com/jobs', 'Example', 'greenhouse', 'active');
INSERT INTO users(user_id, email, status) VALUES (1, 'worker@example.com', 'live');
INSERT INTO job_listings(job_listing_id, canonical_url, board_id, current_version, last_seen_at, posted_at, metadata_json) VALUES (1, 'https://example.com/jobs/engineer', 1, 1, '2025-01-02T03:04:05Z', NULL, '{"source":"fixture"}');
INSERT INTO job_listing_versions(job_listing_id, version, found_at, posted_at, title, salary, body_text, content_hash) VALUES (1, 1, '2025-01-02T03:04:05Z', NULL, 'Engineer', '$100k', 'Build systems', 'fixture-hash-a');
INSERT INTO job_matches(user_id, job_listing_id, job_listing_version, rating, decision, matcher_name, matcher_metadata_json, job_match_data_json) VALUES (1, 1, 1, 5, 'match', 'fixture', '{"model":"fixture"}', '{"reason":"fixture"}');
