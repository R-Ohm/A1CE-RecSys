package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	// Initialize HTTP router
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/api/v1/recommendations", handleRecommendations)
	mux.HandleFunc("/api/v1/student-data", handleStudentData)
	mux.HandleFunc("/api/v1/course-catalog", handleCourseCatalog)
	mux.HandleFunc("/api/v1/health", handleHealth)

	// Wrap with middleware
	handler := corsMiddleware(loggingMiddleware(authMiddleware(mux)))

	// Create server with timeouts
	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("===========================================")
	log.Println("A1CE Course Recommender API Server")
	log.Println("===========================================")
	log.Println("Server listening on: http://localhost:8080")
	log.Println("API Base URL: http://localhost:8080/api/v1")
	log.Println("")
	log.Println("Endpoints:")
	log.Println("  GET  /api/v1/health")
	log.Println("  POST /api/v1/recommendations")
	log.Println("  GET  /api/v1/student-data")
	log.Println("  GET  /api/v1/course-catalog")
	log.Println("===========================================")
	log.Println("CORS: Enabled (all origins)")
	log.Println("Waiting for requests...")
	log.Println("===========================================")

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// CORS middleware - must be FIRST in the chain
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[CORS] Request from Origin: %s", r.Header.Get("Origin"))

		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			log.Printf("[CORS] Preflight request for %s", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Middleware for logging requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("Completed in %v", time.Since(start))
	})
}

// Middleware for JWT authentication
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check
		if r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			sendError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Missing authorization token", "")
			return
		}

		// TODO: Validate JWT token with A1CE
		// For now, just check if Bearer token exists
		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			sendError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Invalid token format", "")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET requests allowed", "")
		return
	}

	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "Course Recommender API",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
		"dependencies": map[string]string{
			"a1ce_api": "connected",
			"database": "not_applicable",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Main recommendations endpoint
func handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST requests allowed", "")
		return
	}

	var req RecommendationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse request body", err.Error())
		return
	}

	// Validate required fields
	if req.StudentID == "" {
		sendError(w, http.StatusBadRequest, "MISSING_REQUIRED_FIELD", "student_id is required", "")
		return
	}
	if req.Semester == "" {
		sendError(w, http.StatusBadRequest, "MISSING_REQUIRED_FIELD", "semester is required", "")
		return
	}

	// Set defaults
	if req.MaxCreditLoad == 0 {
		req.MaxCreditLoad = 60
	}
	if req.MaxSets == 0 {
		req.MaxSets = 1
	}

	// Generate recommendations
	service := NewRecommenderService()

	// Don't forget to add token
	service.a1ceClient.JWTToken = getAuthorzationCred(r, "Bearer")

	result, err := service.GenerateRecommendations(&req)
	if err != nil {
		log.Printf("Error generating recommendations: %v", err)
		sendError(w, http.StatusInternalServerError, "ALGORITHM_ERROR", "Failed to generate recommendations", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Student data endpoint
func handleStudentData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET requests allowed", "")
		return
	}

	studentID := r.URL.Query().Get("student_id")
	if studentID == "" {
		sendError(w, http.StatusBadRequest, "MISSING_REQUIRED_FIELD", "student_id parameter is required", "")
		return
	}

	// Fetch student data
	client := NewA1CEClient()

	// Don't forget to add token
	client.JWTToken = getAuthorzationCred(r, "Bearer")

	profile, err := client.GetStudentProfile(studentID)
	if err != nil {
		log.Printf("Error fetching student data: %v", err)
		sendError(w, http.StatusInternalServerError, "A1CE_API_UNAVAILABLE", "Failed to fetch student data", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// Course catalog endpoint
func handleCourseCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET requests allowed", "")
		return
	}

	semester := r.URL.Query().Get("semester")
	curriculumVersion := r.URL.Query().Get("curriculum_version")

	if semester == "" || curriculumVersion == "" {
		sendError(w, http.StatusBadRequest, "MISSING_REQUIRED_FIELD", "semester and curriculum_version are required", "")
		return
	}

	// Fetch course catalog
	client := NewA1CEClient()
	catalog, err := client.GetCourseCatalog(semester, curriculumVersion)
	if err != nil {
		log.Printf("Error fetching course catalog: %v", err)
		sendError(w, http.StatusInternalServerError, "A1CE_API_UNAVAILABLE", "Failed to fetch course catalog", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(catalog)
}

// Helper function to send error responses
func sendError(w http.ResponseWriter, statusCode int, errorCode, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "error",
		"error_code": errorCode,
		"message":    message,
		"details":    details,
	})
}

func getAuthorzationCred(r *http.Request, target_type string) string {
	raw_auth_vals := strings.Split(r.Header.Get("Authorization"), " ")
	if len(raw_auth_vals) == 2 {
		auth_type := raw_auth_vals[0]
		cred := raw_auth_vals[1]
		if auth_type == target_type {
			return cred
		}
	}
	return ""
}