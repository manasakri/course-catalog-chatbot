package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Chatbot struct holds the courses db information and LLM client
type Chatbot struct {
	client      *openai.Client
	db          *DB
	TotalTokens int
}

// ChatCompletionReq struct to add abstraction to API
type ChatCompletion struct {
	SystemPrompt string
	UserContent  string
	Query        string
}

func NewChatbot(db *DB) *Chatbot {
	api_key := os.Getenv("OPENAI_API_KEY")
	return &Chatbot{
		openai.NewClient(api_key), db, 0,
	}
}

// ChatCompletion allows for generalization of system prompts, so that it can be
// called outside
func (chatbot *Chatbot) ChatCompletion(content ChatCompletion) (string, int, error) {

	// Chat request is created with given prompts as instructions
	req := openai.ChatCompletionRequest{
		Model: openai.GPT5,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: content.SystemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: content.UserContent,
			},
		},
	}

	resp, err := chatbot.client.CreateChatCompletion(context.TODO(), req)
	if err != nil {
		return "", 0, err
	}

	chatbot.TotalTokens += resp.Usage.TotalTokens
	return resp.Choices[0].Message.Content, resp.Usage.TotalTokens, nil
}

// AgenticChat allows the main function loop with context accumulation
func (chatbot *Chatbot) AgenticChat() {
	scanner := bufio.NewScanner(os.Stdin)
	dialogue := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a helpful assistant for the USF course catalog. When users ask about courses, use the query_courses tool to find relevant courses. Provide helpful answers with all relevant course details. Remember context from previous questions in this conversation.",
		},
	}

	question_count := 0

	for {

		// Start of new query for user to enter
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		question := strings.TrimSpace(scanner.Text())

		question_count++
		start_query := time.Now()

		dialogue = append(dialogue, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: question,
		})

		params := ChatCompletion{
			SystemPrompt: "You are a helpful assistant for the USF course catalog. When users ask about courses, use the query_courses tool to find relevant courses. Provide helpful answers with all relevant course details.",
			UserContent:  question,
			Query:        question,
		}

		response, tokens, err := chatbot.ChatCompletionTool(params, dialogue)
		query_dur := time.Since(start_query)

		if err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Println("\n" + response)
			fmt.Printf("\nTokens: %d \n Time: %.2fs \n Total Tokens: %d]\n\n",
				tokens, query_dur.Seconds(), chatbot.TotalTokens)

			dialogue = append(dialogue, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: response,
			})
		}
	}
}

// Chat takes uses the LLM information to answer the users question based on prompts and
// similarity searching (keeping for backward compatibility)
func (chatbot *Chatbot) Chat(query string) {
	results := chatbot.db.Query(query)
	prompt := strings.Join(results, "\n")
	if len(prompt) == 0 {
		println("Error")
		// Returns a slice of strings from "plain"
		return
	}

	system_prompt := "You have been given information about the USF course catalog and should answer any questions that are asked about USF courses, their times, their rooms, and the professor who teaches them. The answers should contain all information, even if the question references a partial name"
	message_content := fmt.Sprintf("Course Data:\n%s\n\nQuestion: %s", prompt, query)

	params := ChatCompletion{
		system_prompt, message_content, query,
	}

	response, tokens, err := chatbot.ChatCompletion(params)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("\n%s\n", response)
	fmt.Printf("Tokens used: %d\n\n", tokens)
	chatbot.TotalTokens += tokens
}
