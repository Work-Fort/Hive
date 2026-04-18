module github.com/Work-Fort/Hive/tests/e2e

go 1.26

replace github.com/Work-Fort/Hive => ../../

replace github.com/Work-Fort/Hive/client => ../../client

require (
	github.com/Work-Fort/Hive v0.0.0-00010101000000-000000000000
	github.com/Work-Fort/Hive/client v0.0.0-00010101000000-000000000000
)
