package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/recommendations", handleRecommendations)
	mux.HandleFunc("/api/v1/student-data", handleStudentData)
	mux.HandleFunc("/api/v1/course-catalog", handleCourseCatalog)
	mux.HandleFunc("/api/v1/health", handleHealth)

	handler := corsMiddleware(loggingMiddleware(authMiddleware(mux)))

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

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "A1CE-Recommender",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func handleStudentData(w http.ResponseWriter, r *http.Request) {
	studentID := r.URL.Query().Get("student_id")
	if studentID == "" {
		sendError(w, http.StatusBadRequest, "MISSING_PARAM", "student_id is required", "")
		return
	}

	client := NewA1CEClient()
	client.JWTToken = getAuthorzationCred(r, "token")

	profile, err := client.GetStudentProfile(studentID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "API_ERROR", "Failed to fetch student data", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func handleCourseCatalog(w http.ResponseWriter, r *http.Request) {
	semester := r.URL.Query().Get("semester")
	curriculumVersionStr := r.URL.Query().Get("curriculum_version")
	curriculumVersion, err := strconv.Atoi(curriculumVersionStr)

	if semester == "" || err != nil {
		sendError(w, http.StatusBadRequest, "MISSING_REQUIRED_FIELD", "semester/version required", "")
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
	json.NewEncoder(w).Encode(catalog)
}

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

	if req.MaxCreditLoad > 60 {
		sendError(w, http.StatusBadRequest, "CREDIT_OVERLOAD", "maximum credit load is at 60, please contact CMKL staff for credit overload", "")
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
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch catalog", err.Error())
		return
	}

	// 4. Calculate Interests
	profile.InterestWeights = make(map[string]float64)
	if len(successfulCourses) > 0 {
		for _, successCode := range successfulCourses {
			parts := strings.Split(successCode, "-")
			if len(parts) > 0 {
				prefix := parts[0]
				for _, course := range catalog.Courses {
					if strings.HasPrefix(course.CourseCode, prefix) {
						profile.InterestWeights[course.SubdomainID] += 5.0
					}
				}
			}
		}
	}
	totalWeight := 0.0
	for _, w := range profile.InterestWeights {
		totalWeight += w
	}
	if totalWeight > 0 {
		for k := range profile.InterestWeights {
			profile.InterestWeights[k] /= totalWeight
		}
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
	curriculumVersionStr := r.URL.Query().Get("curriculum_version")
	curriculumVersion, err := strconv.Atoi(curriculumVersionStr)

	if semester == "" || err != nil {
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
	json.NewEncoder(w).Encode(response)
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
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("Completed in %v", time.Since(start))
	})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
