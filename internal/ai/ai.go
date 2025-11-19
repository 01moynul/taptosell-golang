package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	// REMOVED: iterator import
)

// AIService holds the Gemini client and the read-only database connection.
type AIService struct {
	Client *genai.Client
	DB     *sql.DB
}

// NewAIService initializes the Gemini client.
func NewAIService(apiKey string, dbReadOnly *sql.DB) (*AIService, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return &AIService{Client: client, DB: dbReadOnly}, nil
}

// UPDATED: Now returns (response string, totalTokens int, err error)
func (s *AIService) GenerateResponse(ctx context.Context, userMessage string, userRole string, modelName string) (string, int, error) {
	// 1. Use the model name passed from the handler (dynamic configuration)
	if modelName == "" {
		modelName = "gemini-1.5-flash" // Fallback default
	}
	model := s.Client.GenerativeModel(modelName)

	// 2. Define Tools (Same as before)
	sqlTool := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "run_readonly_sql",
				Description: "Executes a READ-ONLY SQL query (SELECT only) to answer questions.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"query": {
							Type:        genai.TypeString,
							Description: "The MySQL SELECT query to execute.",
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}
	model.Tools = []*genai.Tool{sqlTool}

	// 3. System Instructions (Same schema as before)
	schemaContext := s.getSchemaDefinition()
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(fmt.Sprintf(`
			You are the TapToSell AI Assistant. Role: %s.
			Access: MySQL database (run_readonly_sql).
			Schema: %s
			Rules: SELECT only. Be concise. Map typos (e.g. "frute") to correct tables.
		`, userRole, schemaContext))},
	}

	// 4. Execute Chat
	cs := model.StartChat()
	res, err := cs.SendMessage(ctx, genai.Text(userMessage))
	if err != nil {
		return "", 0, fmt.Errorf("error sending message: %w", err)
	}

	// 5. Handle Response & Count Tokens
	// We need to track tokens across the whole conversation (initial prompt + tool use)
	// Note: Gemini Go SDK UsageMetadata is on the Response object.
	totalTokens := 0
	if res.UsageMetadata != nil {
		totalTokens += int(res.UsageMetadata.TotalTokenCount)
	}

	// Loop for Function Calls
	for {
		if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
			return "No response.", totalTokens, nil
		}
		part := res.Candidates[0].Content.Parts[0]

		funcCall, ok := part.(genai.FunctionCall)
		if !ok {
			// It's text. Return the text and the total tokens.
			return fmt.Sprintf("%v", part), totalTokens, nil
		}

		// Handle SQL Tool
		if funcCall.Name == "run_readonly_sql" {
			args := funcCall.Args
			query, ok := args["query"].(string)
			if !ok {
				return "", totalTokens, fmt.Errorf("invalid query argument")
			}
			log.Printf("🤖 AI running SQL: %s", query)

			sqlResult, sqlErr := s.runReadOnlyQuery(query)
			if sqlErr != nil {
				sqlResult = fmt.Sprintf("SQL Error: %v", sqlErr)
			}

			// Send Tool Response back to Gemini
			res, err = cs.SendMessage(ctx, genai.FunctionResponse{
				Name:     "run_readonly_sql",
				Response: map[string]interface{}{"result": sqlResult},
			})
			if err != nil {
				return "", totalTokens, fmt.Errorf("tool response error: %w", err)
			}

			// Add tokens from this new turn
			if res.UsageMetadata != nil {
				// Note: UsageMetadata in the response is usually cumulative for the request
				// For simplicity in this version, we just take the latest TotalTokenCount if available
				totalTokens = int(res.UsageMetadata.TotalTokenCount)
			}
		} else {
			return "", totalTokens, fmt.Errorf("unknown function: %s", funcCall.Name)
		}
	}
}

// runReadOnlyQuery (Same as before)
func (s *AIService) runReadOnlyQuery(query string) (string, error) {
	normalized := strings.ToUpper(query)
	if strings.Contains(normalized, "UPDATE") || strings.Contains(normalized, "DELETE") || strings.Contains(normalized, "DROP") || strings.Contains(normalized, "INSERT") {
		return "", fmt.Errorf("security violation: modify operations are not allowed")
	}
	rows, err := s.DB.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	columns, _ := rows.Columns()
	count := len(columns)
	tableData := []map[string]interface{}{}
	for rows.Next() {
		values := make([]interface{}, count)
		valuePtrs := make([]interface{}, count)
		for i := range columns {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		entry := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			entry[col] = v
		}
		tableData = append(tableData, entry)
	}
	jsonData, err := json.Marshal(tableData)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

// getSchemaDefinition (Same as before)
func (s *AIService) getSchemaDefinition() string {
	return `
	- users (id, role [dropshipper, supplier, admin], status [unverified, pending, active, suspended], email, full_name, phone_number, company_name, ssm_number, city, state)
	- products (id, supplier_id, name, description, category, brand, price_to_tts, srp, stock_quantity, status [pending_review, active, inactive, rejected], weight_grams)
	- categories (id, name, slug, parent_id)
	- brands (id, name, slug)
	- carts (id, user_id)
	- cart_items (id, cart_id, product_id, quantity)
	- inventory_items (id, user_id, name, sku, price, stock, promoted_product_id)
	- inventory_categories (id, user_id, name, slug)
	- inventory_brands (id, user_id, name, slug)
	- wallet_transactions (id, user_id, type [topup, order_payment, withdrawal, refund, payout], status, amount, balance_after, created_at)
	- withdrawal_requests (id, user_id, amount, status [pending, approved, rejected], bank_details, rejection_reason)
	- price_appeals (id, product_id, supplier_id, old_price, new_price, status, reason)
	- notifications (id, user_id, message, is_read)
	- plans (id, name, price, duration_days, ai_credits_included, is_public)
	- user_subscriptions (id, user_id, plan_id, status, expires_at)
	- ai_user_credits (id, user_id, credits_remaining)
	- ai_chat_history (id, user_id, user_message, ai_response, tokens_used, cost_incurred)
	- settings (setting_key, setting_value, description)
	`
}
