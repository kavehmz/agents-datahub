package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// Simple mock payment service for testing REST API source
func main() {
	http.HandleFunc("/api/payment/history", handlePaymentHistory)
	http.HandleFunc("/health", handleHealth)

	fmt.Println("Mock Payment Service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handlePaymentHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		User string `json:"user"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Mock payment data
	payments := []map[string]interface{}{
		{
			"id":     "pay-001",
			"user":   req.User,
			"amount": 99.99,
			"status": "completed",
			"remark": "Monthly subscription",
			"date":   "2024-01-15",
		},
		{
			"id":     "pay-002",
			"user":   req.User,
			"amount": 49.99,
			"status": "completed",
			"remark": "Additional credits",
			"date":   "2024-01-20",
		},
		{
			"id":     "pay-003",
			"user":   req.User,
			"amount": 149.99,
			"status": "pending",
			"remark": "Premium upgrade",
			"date":   "2024-01-25",
		},
	}

	response := map[string]interface{}{
		"payments": payments,
		"total":    len(payments),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("Payment history request for user: %s", req.User)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
