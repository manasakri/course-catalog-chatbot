package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
	openai "github.com/sashabaranov/go-openai"
)

// DB is a struct that contains db connection and LLM client
type DB struct {
	db     *sql.DB
	client *openai.Client
}

// NewDB uses vector searching to create a new DB connection
// Now supports preloading existing database
func NewDB() *DB {
	name := "courses.db"

	// Remove DB to ensure clean state for tests
	os.Remove(name)

	// Starts and loads sqlite-vec
	sqlite_vec.Auto()

	db, err := sql.Open("sqlite3", name)
	if err != nil {
		log.Fatal(err)
	}

	// Virtual table is created with embeddings with essential fields only
	// sqlite-vec has a limit of 16 metadata columns
	// IF NOT EXISTS ensures we don't recreate if already present
	_, err = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS courses USING vec0(
			id INTEGER PRIMARY KEY,
			department TEXT,
			course_num TEXT,
			course_name TEXT,
			first_name TEXT,
			last_name TEXT,
			email TEXT,
			days TEXT,
			start_time TEXT,
			end_time TEXT,
			building TEXT,
			room TEXT,
			plain TEXT,
			embedding FLOAT[3072]
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	return &DB{
		db, openai.NewClient(os.Getenv("OPENAI_API_KEY")),
	}
}

// Create DB creates vector embeddings based on the input text
func (db *DB) CreateDB(input []string) [][]byte {
	var batch = 1000

	// Uses slice of bytes to hold embeddings(prioritizes storage)
	var results [][]byte

	for i := 0; i < len(input); i += batch {
		end := i + batch
		if end > len(input) {
			end = len(input)
		}

		batch := input[i:end]

		req := openai.EmbeddingRequest{
			Input: batch,
			Model: openai.LargeEmbedding3,
		}
		resp, err := db.client.CreateEmbeddings(context.TODO(), req)
		if err != nil {
			log.Fatal(err)
		}

		// Uses slice of bytes to hold embeddings (prioritizes storage)
		for _, data := range resp.Data {
			embedding, err := sqlite_vec.SerializeFloat32(data.Embedding)
			if err != nil {
				log.Fatal(err)
			}
			results = append(results, embedding)
		}
	}

	return results
}

// InsertDB adds each course to the DB with essential fields
func (db *DB) InsertDB(rowid int, row []string, bytes_embedding []byte) {
	if len(row) < 21 {
		return
	}

	// Parse essential fields from CSV
	// Index mapping based on: CS,221,01,40438,L,M,C and Systems Programming,In-Person,IP,MW,0920,1105,8/19/25,12/3/25,HR,148,24,Paul,Haskell,phaskell@usfca.edu,SC
	department := row[0]  // CS
	course_num := row[1]  // 221
	course_name := row[6] // C and Systems Programming
	first_name := row[17] // Paul
	last_name := row[18]  // Haskell
	email := row[19]      // phaskell@usfca.edu
	days := row[9]        // MW
	start_time := row[10] // 0920
	end_time := row[11]   // 1105
	building := row[14]   // HR
	room := row[15]       // 148

	// Create plain text representation (all fields for searching)
	plain := strings.Join(row, ",")

	_, err := db.db.Exec(`
		INSERT INTO courses (
			id, department, course_num, course_name, first_name, last_name,
			email, days, start_time, end_time, building, room, plain, embedding
		) 
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, rowid, department, course_num, course_name, first_name, last_name,
		email, days, start_time, end_time, building, room, plain, bytes_embedding)
	if err != nil {
		log.Fatal(err)
	}
}

// Query uses similarity searching to get the most similar or "closest" result
func (course_db *DB) Query(query string) []string {
	embeddings := course_db.CreateDB([]string{query})
	embedding := embeddings[0]

	rows, err := course_db.db.Query(`
		SELECT plain FROM courses 
		WHERE embedding MATCH ? 
		ORDER BY distance 
		LIMIT 20
	`, embedding)

	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var plain string
		err = rows.Scan(&plain)
		if err != nil {
			log.Fatal(err)
		}
		results = append(results, plain)
	}
	return results
}

// QueryCourses uses a vector search to return the course info in a certain order
func (course_db *DB) QueryCourses(query string) []Course {
	embeddings := course_db.CreateDB([]string{query})
	embedding := embeddings[0]

	rows, err := course_db.db.Query(`
        SELECT department, course_num, course_name, first_name, last_name, email, days, start_time, end_time, building, room
        FROM courses
        WHERE embedding MATCH ?
        ORDER BY distance
        LIMIT 20
    `, embedding)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	// Takes the embeddings from SQL DB and converts them into slices that contain the Course struct info
	var courses []Course
	for rows.Next() {
		var course Course
		var first_name, last_name, start_time, end_time, building, room string

		// Creates new Course struct to hold data from current row
		if err := rows.Scan(&course.Department, &course.CourseNum, &course.CourseName,
			&first_name, &last_name, &course.Email, &course.Days,
			&start_time, &end_time, &building, &room); err != nil {
			log.Fatal(err)
		}

		// Combine fields
		course.Instructor = first_name + " " + last_name
		course.Time = start_time + "-" + end_time
		course.Location = building + " " + room

		courses = append(courses, course)
	}
	return courses
}

// QueryByField is a generic function to query courses by any field
func (course_db *DB) QueryByField(field, value string) []Course {
	var query string
	var args []interface{}

	switch field {
	case "department":
		query = `
			SELECT department, course_num, course_name, first_name, last_name, email, days, start_time, end_time, building, room
			FROM courses
			WHERE UPPER(department) = UPPER(?)
			ORDER BY course_num`
		args = []interface{}{value}

	case "instructor":
		query = `
			SELECT department, course_num, course_name, first_name, last_name, email, days, start_time, end_time, building, room
			FROM courses
			WHERE UPPER(first_name) LIKE UPPER(?) OR UPPER(last_name) LIKE UPPER(?)
			ORDER BY department, course_num`
		args = []interface{}{"%" + value + "%", "%" + value + "%"}

	case "location":
		query = `
			SELECT department, course_num, course_name, first_name, last_name, email, days, start_time, end_time, building, room
			FROM courses
			WHERE UPPER(building) LIKE UPPER(?) OR UPPER(room) LIKE UPPER(?)
			ORDER BY department, course_num`
		args = []interface{}{"%" + value + "%", "%" + value + "%"}

	default:
		return []Course{}
	}

	rows, err := course_db.db.Query(query, args...)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var courses []Course
	for rows.Next() {
		var course Course
		var first_name, last_name, start_time, end_time, building, room string

		if err := rows.Scan(&course.Department, &course.CourseNum, &course.CourseName,
			&first_name, &last_name, &course.Email, &course.Days,
			&start_time, &end_time, &building, &room); err != nil {
			log.Fatal(err)
		}

		course.Instructor = first_name + " " + last_name
		course.Time = start_time + "-" + end_time
		course.Location = building + " " + room

		courses = append(courses, course)
	}
	return courses
}
