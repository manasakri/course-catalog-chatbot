package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

var db *DB
var chatbot *Chatbot

func TestMain(m *testing.M) {
	db = NewDB()
	chatbot = NewChatbot(db)

	file, _ := os.Open("courses.csv")
	reader := csv.NewReader(file)
	courses, _ := reader.ReadAll()
	file.Close()

	course_data := courses[1:]

	load_batch := 1000
	loaded := 0

	for start := 0; start < len(course_data); start += load_batch {
		end := min(start+load_batch, len(course_data))
		batch := course_data[start:end]

		var texts []string
		for _, row := range batch {
			if len(row) >= 21 {
				texts = append(texts, strings.Join(row, " "))
			}
		}

		if len(texts) > 0 {
			embeddings := db.CreateDB(texts)

			idx := 0
			for row_idx, row := range batch {
				if len(row) >= 21 {
					db.InsertDB(start+row_idx+1, row, embeddings[idx])
					idx++
					loaded++
				}
			}
		}
	}

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM courses").Scan(&count)

	fmt.Println("DB Loaded.")

	m.Run()
}

func TestProject06(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		keywords []string
	}{
		{
			name:     "TestCS272",
			query:    "Who is teaching CS 272?",
			keywords: []string{"Phil Peterson", "CS 272"},
		},

		{
			name:     "TestEmail",
			query:    "What's his email address?",
			keywords: []string{"email", "@"},
		},
		{
			name:     "TestPhilGreg",
			query:    "What are Phil Peterson and Greg Benson teaching?",
			keywords: []string{"Peterson", "Benson"},
		},
		{
			name:     "TestHR148",
			query:    "Which courses is Phil Peterson teaching in HR 148?",
			keywords: []string{"HR", "148"},
		},
		{
			name:     "TestKHall",
			query:    "Which department's courses are most frequently scheduled in Kalmanovitz (KA) Hall?",
			keywords: []string{"(KA)", "Hall"},
		},
	}

	dialogue := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a helpful assistant for the USF course catalog. When users ask about courses, use the query_courses tool to find relevant courses. Always include the course department and number (e.g., CS 272) along with the instructor name in your answers.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			dialogue = append(dialogue, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: test.query,
			})

			params := ChatCompletion{
				SystemPrompt: "You are a helpful assistant for the USF course catalog. Always include course department and number (e.g., CS 272) in your answers.",
				UserContent:  test.query,
				Query:        test.query,
			}

			response, _, err := chatbot.ChatCompletionTool(params, dialogue)
			if err != nil {
				t.Errorf("Error: %v", err)
				return
			}

			dialogue = append(dialogue, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: response,
			})

			found := false
			for _, keyword := range test.keywords {
				if strings.Contains(strings.ToUpper(response), strings.ToUpper(keyword)) {
					found = true
					break
				}
			}

			if found {
				t.Logf("Test Case Passed")
				fmt.Println("Response: ", response)
			} else {
				t.Errorf("Test Case Failed")
				fmt.Println("Response: ", response)

			}
		})
	}
}
