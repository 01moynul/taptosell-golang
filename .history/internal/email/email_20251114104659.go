package email

import (
	"fmt" // For formatting strings
	"log" // For printing to the console
)

// SendEmail is our placeholder email function.
// In the future, this will use a real email API (like SendGrid).
func SendEmail(to string, subject string, body string) error {
	// --- THIS IS OUR PLACEHOLDER ---
	// Instead of sending a real email, we log it to the console.
	// This lets us "see" the email and test our code without an API key.
	log.Println("====================================================")
	log.Printf("--- NEW EMAIL (PLACEHOLDER) ---")
	log.Printf("To: %s", to)
	log.Printf("Subject: %s", subject)
	log.Println("--- Body ---")
	log.Println(body)
	log.Println("====================================================")

	return nil // Assume success for now
}

// SendVerificationEmail is a helper that uses our main SendEmail function.
func SendVerificationEmail(to string, code string) error {
	subject := "Verify your TapToSell Account"

	// We create a simple text body for the email.
	body := fmt.Sprintf(
		"Welcome to TapToSell!\n\nYour verification code is: %s\n\nThis code will expire in 15 minutes.",
		code,
	)

	// Call our main email sender
	return SendEmail(to, subject, body)
}
