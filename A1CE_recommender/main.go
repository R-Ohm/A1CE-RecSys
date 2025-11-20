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

	client := NewA1CEClient()
	client.JWTToken = getAuthorzationCred(r, "token")

	catalog, err := client.GetCourseCatalog(semester, curriculumVersion)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch catalog", err.Error())
		return
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

	client := NewA1CEClient()
	client.JWTToken = getAuthorzationCred(r, "token")

	// 1. Fetch Profile
	profile, err := client.GetStudentProfile(req.StudentID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch profile", err.Error())
		return
	}

	// 2. Identify Successful Courses & Populate Exclusion List
	var successfulCourses []string

	if req.PreviousSemester == "ALL" {
		log.Printf("Calculating interest based on ALL past courses")
		// In OVERALL mode, any course with a grade is considered "taken" and should be excluded
		for courseCode, grade := range profile.Competencies {
			// Add to exclusion list if not already there
			if !containsString(profile.CompletedCourses, courseCode) {
				profile.CompletedCourses = append(profile.CompletedCourses, courseCode)
			}

			if grade > 1.0 {
				successfulCourses = append(successfulCourses, courseCode)
			}
		}
	} else if req.PreviousSemester != "" {
		log.Printf("Looking for successes in semester: %s", req.PreviousSemester)
		semesterCards, err := client.GetSemesterCompetencies(req.StudentID, req.PreviousSemester)
		if err == nil {
			for _, card := range semesterCards {
				// Ensure semester specific courses are also marked completed
				if !containsString(profile.CompletedCourses, card.CourseCode) {
					profile.CompletedCourses = append(profile.CompletedCourses, card.CourseCode)
				}

				if card.Grade > 1.0 {
					successfulCourses = append(successfulCourses, card.CourseCode)
				}
			}
		}
	}

	// 3. Fetch Catalog
	catalog, err := client.GetCourseCatalog(req.Semester, profile.CurriculumVersion)
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

	// 5. Score Courses
	requirements := &CurriculumRequirements{
		CurriculumVersion:        profile.CurriculumVersion,
		RequiredCompetencies:     profile.RequiredCompetencies,
		DistributionRequirements: make(map[string]float64),
		TotalCreditsRequired:     float64(profile.TotalCredits.Required),
	}
	for k, v := range profile.DistributionCredits {
		requirements.DistributionRequirements[k] = float64(v.Required)
	}

	var scoredCourses []RecommendedCourse
	for _, course := range catalog.Courses {
		if !CheckPrerequisites(course, profile) {
			continue
		}

		// STRICT FILTER: Check against the fully populated CompletedCourses list
		if isCourseCompleted(course, profile) {
			continue
		}

		compScore := CalculateCompetencyMatchScore(course, profile)
		interestScore := CalculateInterestScore(course, profile)
		progScore := CalculateProgramProgressScore(course, profile, requirements)

		fitScore := 0.2*compScore + 0.6*interestScore + 0.2*progScore

		scoredCourses = append(scoredCourses, RecommendedCourse{
			Course:                 course,
			FitScore:               fitScore,
			MatchedCompetencies:    GetMatchedCompetencies(course, profile),
			MissingCompetencies:    GetMissingCompetencies(course, profile),
			CompetencyMatchScore:   compScore,
			InterestAlignmentScore: interestScore,
			ProgramProgressScore:   progScore,
			Reason:                 fmt.Sprintf("Interest Score: %.2f", interestScore),
		})
	}

	for i := 0; i < len(scoredCourses); i++ {
		for j := i + 1; j < len(scoredCourses); j++ {
			if scoredCourses[i].FitScore < scoredCourses[j].FitScore {
				scoredCourses[i], scoredCourses[j] = scoredCourses[j], scoredCourses[i]
			}
		}
	}

	recommendedSet := OptimizeCourseSet(scoredCourses, profile, requirements, req.MaxCreditLoad)

	totalCredits := 0.0
	for _, r := range recommendedSet {
		totalCredits += r.Course.CreditHours
	}

	response := RecommendationSet{
		StudentID:      req.StudentID,
		Semester:       req.Semester,
		RecommendedSet: recommendedSet,
		TotalCredits:   totalCredits,
		Metrics:        EvaluationMetrics{GoodnessScore: 0.85},
		Metadata: RecommendationMetadata{
			GenerationTimestamp: time.Now(),
			AlgorithmVersion:    "1.9-StrictFilter",
		},
		Status: "success",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper to check if a course is already completed
func isCourseCompleted(course Course, profile *StudentProfile) bool {
	for _, completedCode := range profile.CompletedCourses {
		// Strict check against ID, Code, and Name
		if completedCode == course.CourseID {
			return true
		}
		if course.CourseCode != "" && completedCode == course.CourseCode {
			return true
		}
		if completedCode == course.CourseName {
			return true
		}
	}
	return false
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

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
