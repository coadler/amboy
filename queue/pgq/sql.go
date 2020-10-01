package pgq

const bootstrapDB = `
CREATE TABLE IF NOT EXISTS jobs (
id text NOT NULL PRIMARY KEY,
type text NOT NULL,
queue_group text DEFAULT ''::text NOT NULL,
version integer NOT NULL,
priority integer  NOT NULL
);

CREATE TABLE IF NOT EXISTS job_body (
id text NOT NULL PRIMARY KEY,
job jsonb NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS job_scopes (
id text NOT NULL,
scope text UNIQUE NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS job_status (
id text NOT NULL PRIMARY KEY,
owner text NOT NULL,
completed boolean NOT NULL,
in_progress boolean NOT NULL,
mod_ts timestamptz NOT NULL,
mod_count integer NOT NULL,
err_count integer NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS job_errors (
id text NOT NULL,
error text NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS job_time (
id text NOT NULL PRIMARY KEY,
created timestamptz NOT NULL,
started timestamptz NOT NULL,
ended timestamptz NOT NULL,
wait_until timestamptz NOT NULL,
dispatch_by timestamptz NOT NULL,
max_time integer NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS dependency (
id text NOT NULL PRIMARY KEY,
dep_type text NOT NULL,
dep_version integer NOT NULL,
dependency jsonb NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS dependency_edges (
id text NOT NULL NOT NULL,
edge text NOT NULL,
FOREIGN KEY (id) REFERENCES jobs(id) ON DELETE CASCADE
);
`

const getActiveGroups = `
SELECT
   DISTINCT queue_group
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
   status.completed = false
   OR (status.completed = true AND status.mod_ts >= $1)
`

const getJobByID = `
SELECT
   *
FROM
   jobs
   INNER JOIN job_body AS job ON jobs.id=job.id
   INNER JOIN job_status AS status ON jobs.id=status.id
   INNER JOIN job_time AS time_info ON jobs.id=time_info.id
   INNER JOIN dependency AS dependency ON jobs.id=dependency.id
WHERE
   jobs.id = $1
`

const getErrorsForJob = `
SELECT
   error
FROM
   job_errors
WHERE
   id = $1
`

const updateJob = `
UPDATE
   jobs
SET
   type = :type,
   queue_group = :queue_group,
   version = :version,
   priority = :priority
WHERE
   id = :id
`

const updateJobBody = `
UPDATE
   job_body
SET
   job = :job
WHERE
   id = :id
`

const updateJobStatus = `
UPDATE
   job_status
SET
   owner = :owner,
   completed = :completed,
   in_progress = :in_progress,
   mod_ts = :mod_ts,
   mod_count = :mod_count,
   err_count = :err_count
WHERE
   id = :id`

const updateJobTimeInfo = `
UPDATE
   job_time
SET
   created = :created,
   started = :started,
   ended = :ended,
   wait_until = :wait_until,
   dispatch_by = :dispatch_by,
   max_time = :max_time
WHERE
   id = :id`

const completeSinglePendingJob = `
UPDATE
   job_status
SET
   completed = true,
   in_progress = false,
   mod_count = mod_count + 3
WHERE
   id = $1`

const completeManyPendingJobs = `
UPDATE
   job_status
SET
   completed = true,
   in_progress = false,
   mod_count = mod_count + 3
WHERE
   id IN (?)`

const removeJobScopes = `
DELETE FROM
   job_scopes
WHERE
   id = $1`

const removeManyJobScopes = `
DELETE FROM
   job_scopes
WHERE
   id IN (?)`

const findJobsToCompleteTemplate = `
SELECT
   jobs.id
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
`

const checkCanUpdate = `
SELECT
   COUNT(*)
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
   jobs.id = :id
   AND (
    (status.owner = :owner
     AND status.mod_count = :mod_count - 1
     AND status.mod_ts > :lock_timeout)
    OR status.mod_ts <= :lock_timeout)`

const countTotalJobs = `
SELECT
   COUNT(*)
FROM
   jobs
WHERE
   jobs.queue_group = $1`

const countPendingJobs = `
SELECT
   COUNT(*)
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
   queue_group = $1
   AND status.completed = false`

const countInProgJobs = `
SELECT
   COUNT(*)
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
   queue_group = $1
   AND status.completed = false
   AND status.in_progress = true`

const getAllJobIDs = `
SELECT
   id
FROM
   job_status AS status
ORDER BY
   status.mod_ts DESC`

const getNextJobsBasic = `
SELECT
   jobs.id
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
   status.completed = false
   AND queue_group = :group_name
   AND ((status.in_progress = false) OR (status.in_progress = true AND status.mod_ts <= :lock_expires))
`

const getNextJobsTimingTemplate = `
SELECT
   jobs.id
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
   INNER JOIN job_time AS time_info ON jobs.id=time_info.id
WHERE
   status.completed = false
   AND queue_group = :group_name
   AND ((status.in_progress = false) OR (status.in_progress = true AND status.mod_ts <= :lock_expires))
`

const groupJobStatusTemplate = `
SELECT
   COUNT(jobs.id) AS count,
   type{{project_group}}
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id`

const findJobIDsByStateTemplate = `
SELECT
   jobs.id
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
WHERE
   type = :job_type
`

const recentTimingTemplate = `
SELECT
   jobs.type, {{project_group}}
   AVG((EXTRACT(epoch FROM {{from_time}}) -  EXTRACT(epoch FROM {{to_time}})) * 1000000000) AS duration
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
   INNER JOIN job_time AS time_info ON jobs.id=time_info.id
WHERE
`

const recentJobErrorsTemplate = `
SELECT
   jobs.type, {{queue_group}}
   COUNT(jobs.id) as count,
   SUM(status.err_count) as total,
   AVG(status.err_count) as average{{agg_errors}}
FROM
   jobs
   INNER JOIN job_status AS status ON jobs.id=status.id
   INNER JOIN job_time AS time_info ON jobs.id=time_info.id
   INNER JOIN job_errors as job_errors ON jobs.id=job_errors.id
WHERE
   status.completed = true
   AND status.err_count > 0
   AND time_info.ended > :window
`
