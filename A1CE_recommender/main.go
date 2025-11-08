package main

import (
	"encoding/json"
	"log"
	"net/http"
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

	// Create server with timeouts
	server := &http.Server{
		Addr:         ":8080",
		Handler:      loggingMiddleware(authMiddleware(mux)),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("Starting A1CE Course Recommender API on port 8080...")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
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