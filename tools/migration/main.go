// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// package main provides a simple tool for migrating data from an Erlang
// term format dump to a SQL database.
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	_ "github.com/lib/pq" // Postgres driver
)

// Validator connects to a database to verify the integrity of migrated data.
type Validator struct {
	db *sql.DB
}

// NewValidator creates a new Validator and connects to the database.
func NewValidator(connStr string) (*Validator, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("could not ping database: %w", err)
	}
	return &Validator{db: db}, nil
}

// Close closes the database connection.
func (v *Validator) Close() {
	if v.db != nil {
		v.db.Close()
	}
}

// ValidateRowCount checks if the row count in a table matches the expected count.
func (v *Validator) ValidateRowCount(tableName string, expectedCount int) error {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err := v.db.QueryRow(query).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query row count for table %s: %w", tableName, err)
	}
	if count != expectedCount {
		return fmt.Errorf("row count mismatch for table %s: got %d, want %d", tableName, count, expectedCount)
	}
	log.Printf("Validation successful for table %s: row count is %d.", tableName, count)
	return nil
}

func main() {
	// --- Part 1: Generate migration.sql ---
	log.Println("Generating migration.sql from data dump...")
	if err := generateMigrationSQL(); err != nil {
		log.Fatalf("Failed to generate migration SQL: %v", err)
	}
	log.Println("Successfully generated migration.sql")

	// --- Part 2: Validate the data (Proof-of-Concept) ---
	log.Println("\n--- Data Validation (Proof-of-Concept) ---")
	log.Println("This step demonstrates how a validator would work.")
	log.Println("NOTE: It will fail if you have not set up a Postgres database and the 'POSTGRES_CONN_STR' environment variable.")

	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		log.Println("POSTGRES_CONN_STR not set, skipping validator.")
		return
	}

	validator, err := NewValidator(connStr)
	if err != nil {
		log.Fatalf("Failed to create validator: %v", err)
	}
	defer validator.Close()

	log.Println("Validator created. Running checks...")
	if err := validator.ValidateRowCount("mqtt_sessions", 3); err != nil {
		log.Printf("Validation failed: %v", err)
	}
	if err := validator.ValidateRowCount("mqtt_subscriptions", 4); err != nil {
		log.Printf("Validation failed: %v", err)
	}
}

func generateMigrationSQL() error {
	// Open the Mnesia dump file
	dumpFile, err := os.Open("tests/fixtures/mnesia_dump.txt")
	if err != nil {
		return fmt.Errorf("failed to open mnesia dump file: %w", err)
	}
	defer dumpFile.Close()

	// Create the output SQL file
	sqlFile, err := os.Create("migration.sql")
	if err != nil {
		return fmt.Errorf("failed to create migration.sql: %w", err)
	}
	defer sqlFile.Close()

	// Regex to identify the table name and the rest of the line
	lineRegex := regexp.MustCompile(`^{\s*([a-z_]+)\s*,(.*)}\.$`)

	scanner := bufio.NewScanner(dumpFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "%%") || line == "" {
			continue // Skip comments and empty lines
		}

		matches := lineRegex.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}
		tableName := matches[1]
		data := matches[2]

		var sqlStatement string
		switch tableName {
		case "emqx_session":
			sqlStatement = parseSession(data)
		case "emqx_suboption":
			sqlStatement = parseSubscription(data)
		case "emqx_acl":
			sqlStatement = parseACL(data)
		}

		if sqlStatement != "" {
			if _, err := sqlFile.WriteString(sqlStatement + "\n"); err != nil {
				return fmt.Errorf("failed to write to migration.sql: %w", err)
			}
		}
	}

	return scanner.Err()
}

// NOTE: The following parsing functions are simplified and rely on the specific
// format of the provided dump file. A production-grade tool would require a
// proper Erlang term parser.

func parseSession(data string) string {
	// Example: <<"client_001">>, {session, <<"client_001">>, true, 60, #{}, #{}, [], 1695811200}
	parts := strings.SplitN(data, ",", 2)
	if len(parts) < 2 {
		return ""
	}
	clientID := extractBinaryString(parts[0])

	// Extract values from the tuple
	valueTuple := strings.Trim(parts[1], " {}")
	valueParts := strings.Split(valueTuple, ", ")
	if len(valueParts) < 8 || valueParts[0] != "session" {
		return ""
	}

	cleanStart := valueParts[2]
	keepAlive := valueParts[3]
	createdAt := strings.TrimRight(valueParts[7], "}")

	return fmt.Sprintf("INSERT INTO mqtt_sessions (client_id, clean_start, keepalive_interval, created_at) VALUES ('%s', %s, %s, %s);",
		clientID, cleanStart, keepAlive, createdAt)
}

func parseSubscription(data string) string {
	// Example: {<<"topic/sensor/+">>, <<"client_001">>}, {suboption, 1, false, false, 0}
	parts := strings.SplitN(data, "},", 2)
	if len(parts) < 2 {
		return ""
	}

	keyTuple := strings.Trim(parts[0], " {}")
	keyParts := strings.Split(keyTuple, ", ")
	topicFilter := extractBinaryString(keyParts[0])
	clientID := extractBinaryString(keyParts[1])

	valueTuple := strings.Trim(parts[1], " {}")
	valueParts := strings.Split(valueTuple, ", ")
	if len(valueParts) < 5 || valueParts[0] != "suboption" {
		return ""
	}
	qos := valueParts[1]
	noLocal := valueParts[2]
	retainAsPublished := valueParts[3]

	return fmt.Sprintf("INSERT INTO mqtt_subscriptions (topic_filter, client_id, qos, no_local, retain_as_published) VALUES ('%s', '%s', %s, %s, %s);",
		topicFilter, clientID, qos, noLocal, retainAsPublished)
}

func parseACL(data string) string {
	// Example: {<<"client_001">>, publish}, {acl, allow, <<"topic/sensor/+">>, all}
	parts := strings.SplitN(data, "},", 2)
	if len(parts) < 2 {
		return ""
	}

	keyTuple := strings.Trim(parts[0], " {}")
	keyParts := strings.Split(keyTuple, ", ")
	clientID := extractBinaryString(keyParts[0])
	action := keyParts[1]

	valueTuple := strings.Trim(parts[1], " {}")
	valueParts := strings.Split(valueTuple, ", ")
	if len(valueParts) < 4 || valueParts[0] != "acl" {
		return ""
	}
	permission := valueParts[1]
	topicFilter := extractBinaryString(valueParts[2])

	return fmt.Sprintf("INSERT INTO mqtt_acl_rules (client_id, action, permission, topic_filter) VALUES ('%s', '%s', '%s', '%s');",
		clientID, action, permission, topicFilter)
}

// extractBinaryString is a helper to get the content of an Erlang binary string like <<"content">>.
func extractBinaryString(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<<\"") && strings.HasSuffix(s, "\">>") {
		return s[3 : len(s)-2]
	}
	return s
}