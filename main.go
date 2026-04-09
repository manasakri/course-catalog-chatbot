package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	start_time := time.Now()

	db := NewDB()
	chatbot := NewChatbot(db)

	// Check if database already exists and has data
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM courses").Scan(&count)

	if count == 0 {
		fmt.Println("Loading courses")

		file, err := os.Open("courses.csv")
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		reader := csv.NewReader(file)
		courses, err := reader.ReadAll()
		file.Close()

		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		course_data := courses[1:]
		load_batch := 1000
		total_courses := 0

		// Loads the courses using batching(1000 at a time)
		for start := 0; start < len(course_data); start += load_batch {

			// min returns the smallest number of two
			end := min(start+load_batch, len(course_data))
			batch := course_data[start:end]

			// Gets all the text from each of the rows in the batch
			var texts []string
			for _, row := range batch {
				if len(row) >= 21 {
					texts = append(texts, strings.Join(row, " "))
				}
			}

			// Creates embeddings for the whole batch and inserts each course into the DB
			embeddings := db.CreateDB(texts)

			idx := 0
			for row_idx, row := range batch {
				if len(row) >= 21 {
					db.InsertDB(start+row_idx+1, row, embeddings[idx])
					idx++
					total_courses++
				}
			}

		}

	}

	fmt.Println("Type your questions below:")

	// Allows for the conversation to continue in a loop, in order to imitate human-like conversations
	chatbot.AgenticChat()

	total_duration := time.Since(start_time)
	fmt.Printf("Total Runtime: %.2f seconds\n", total_duration.Seconds())
	fmt.Printf("Total Tokens Used: %d\n", chatbot.TotalTokens)
	fmt.Printf("End Time: %s\n", time.Now().Format("15:04:05"))
}
