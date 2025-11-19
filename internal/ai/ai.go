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
	// REMOVED: "google.golang.org/api/iterator" to fix the unused import error
)

// AIService holds the Gemini client and the read-only database connection.
type AIService struct {
	Client *genai.Client
	DB     *sql.DB // This must be the Read-Only connection
}

// NewAIService initializes the Gemini client and stores the read-only DB connection.
func NewAIService(apiKey string, dbReadOnly *sql.DB) (*AIService, error) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &AIService{
		Client: client,
		DB:     dbReadOnly,
	}, nil
}

// GenerateResponse handles the chat interaction.
func (s *AIService) GenerateResponse(ctx context.Context, userMessage string, userRole string) (string, error) {
	// 1. Select the Model
	model := s.Client.GenerativeModel("gemini-1.5-flash")

	// 2. Define the SQL Tool
	sqlTool := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "run_readonly_sql",
				Description: "Executes a READ-ONLY SQL query (SELECT only) to answer questions about products, users, or inventory.",
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

	// 3. Configure System Instructions (The "Brain")
	// We inject the Schema Definition so the AI knows specific table/column names.
	schemaContext := s.getSchemaDefinition()

	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(fmt.Sprintf(`
			You are the TapToSell AI Assistant. 
			Your role is: %s.
			
			You have access to a MySQL database via the 'run_readonly_sql' tool.
			
			DATABASE SCHEMA (Strictly follow these table and column names):
			%s

			RULES:
			1. ONLY use SELECT statements. NEVER use INSERT, UPDATE, or DELETE.
			2. If the user asks for data, write a SQL query to fetch it using the schema above.
			3. Be concise.
			4. If the user makes a typo (e.g. "frute"), map it to the correct table ("products") based on the schema.
		`, userRole, schemaContext))},
	}

	// 4. Start Chat & Send Message
	cs := model.StartChat()
	res, err := cs.SendMessage(ctx, genai.Text(userMessage))
	if err != nil {
		return "", fmt.Errorf("error sending message to Gemini: %w", err)
	}

	// 5. Handle Tool Calls Loop
	for {
		if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
			return "No response from AI.", nil
		}
		part := res.Candidates[0].Content.Parts[0]

		funcCall, ok := part.(genai.FunctionCall)
		if !ok {
			// It's text, return it.
			return fmt.Sprintf("%v", part), nil
		}

		if funcCall.Name == "run_readonly_sql" {
			args := funcCall.Args
			query, ok := args["query"].(string)
			if !ok {
				return "", fmt.Errorf("invalid query argument")
			}

			log.Printf("🤖 AI running SQL: %s", query)

			sqlResult, sqlErr := s.runReadOnlyQuery(query)
			if sqlErr != nil {
				// Send error back to AI so it can try again
				sqlResult = fmt.Sprintf("SQL Error: %v", sqlErr)
			}

			res, err = cs.SendMessage(ctx, genai.FunctionResponse{
				Name: "run_readonly_sql",
				Response: map[string]interface{}{
					"result": sqlResult,
				},
			})
			if err != nil {
				return "", fmt.Errorf("error sending tool response: %w", err)
			}
		} else {
			return "", fmt.Errorf("unknown function call: %s", funcCall.Name)
		}
	}
}

// runReadOnlyQuery executes the query on the restricted database connection.
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

// getSchemaDefinition provides the "Brain" with the map of the database.
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
