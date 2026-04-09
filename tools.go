package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type Course struct {
	Department string `json:"department"`
	CourseNum  string `json:"course_num"`
	CourseName string `json:"course_name"`
	Instructor string `json:"instructor"`
	Days       string `json:"days"`
	Email      string `json:"email"`
	Time       string `json:"time"`
	Location   string `json:"location"`
}

// ChatCompletionWithTool handles tool calling for course data
func (chatbot *Chatbot) ChatCompletionTool(content ChatCompletion, dialogue []openai.ChatCompletionMessage) (string, int, error) {

	// Gets specialized tools for different query types
	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "query_courses",
				Description: "Search the USF course catalog for courses matching the query using semantic search",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"search_query": {
							Type:        jsonschema.String,
							Description: "The search query to find matching courses",
						},
					},
					Required: []string{"search_query"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "query_by_department",
				Description: "Search for all courses in the specific department. For exampe: CS, MATH, ENGL",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"department": {
							Type:        jsonschema.String,
							Description: "The department code For example: CS, MATH, ENGL",
						},
					},
					Required: []string{"department"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "query_by_instructor",
				Description: "Search for all courses taught by a specific instructor/s",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"instructor": {
							Type:        jsonschema.String,
							Description: "The instructor's name with all their information, for example: first, last, or partial/shortened name",
						},
					},
					Required: []string{"instructor"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "query_by_location",
				Description: "Search for all courses in a specific building or room",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"location": {
							Type:        jsonschema.String,
							Description: "The building name, code, or room number. For example: HR 148, Kalmanovitz Hall",
						},
					},
					Required: []string{"location"},
				},
			},
		},
	}

	client := chatbot.client
	tokens := 0

	// Prevent infinite loops
	max_iteration := 10
	for i := 0; i < max_iteration; i++ {
		req := openai.ChatCompletionRequest{
			Model:    openai.GPT5,
			Messages: dialogue,
			Tools:    tools,
		}

		resp, err := client.CreateChatCompletion(context.TODO(), req)
		if err != nil {
			return "", tokens, fmt.Errorf("Error : %w", err)
		}

		tokens += resp.Usage.TotalTokens
		chatbot.TotalTokens += resp.Usage.TotalTokens
		msg := resp.Choices[0].Message

		dialogue = append(dialogue, msg)

		if len(msg.ToolCalls) == 0 {
			return msg.Content, tokens, nil
		}

		// Handle multiple tool calls
		for _, tool_call := range msg.ToolCalls {
			var args map[string]string
			if err := json.Unmarshal([]byte(tool_call.Function.Arguments), &args); err != nil {
				return "", tokens, fmt.Errorf("Error: %w", err)
			}

			var courses []Course
			tool_name := tool_call.Function.Name

			// Map tool names to database field names
			field_map := map[string]string{
				"query_by_department": "department",
				"query_by_instructor": "instructor",
				"query_by_location":   "location",
			}

			if field, exists := field_map[tool_name]; exists {
				courses = chatbot.db.QueryByField(field, args[field])
			} else {
				results := chatbot.db.Query(args["search_query"])
				courses = CourseResult(results)
			}

			result := map[string]interface{}{
				"courses":    courses,
				"tool_used":  tool_name,
				"query_args": args,
			}
			json_result, err := json.Marshal(result)
			if err != nil {
				return "", tokens, fmt.Errorf("Error: : %w", err)
			}

			tool_resp := openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tool_call.ID,
				Content:    string(json_result),
			}

			dialogue = append(dialogue, tool_resp)
		}
	}

	return "", tokens, fmt.Errorf("Error ")
}

// CourseResult parses course results from plain text catagory
func CourseResult(results []string) []Course {
	var courses []Course
	for _, course_str := range results {
		parts := strings.Split(course_str, ",")
		if len(parts) >= 21 {

			course := Course{
				Department: parts[0],
				CourseNum:  parts[1],
				CourseName: parts[6],
				Instructor: parts[17] + " " + parts[18],
				Email:      parts[19],
				Days:       parts[9],
				Time:       parts[10] + "-" + parts[11],
				Location:   parts[14] + " " + parts[15],
			}
			courses = append(courses, course)
		}
	}
	return courses
}
