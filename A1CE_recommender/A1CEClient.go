package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type A1CEClient struct {
	BaseURL    string
	HTTPClient *http.Client
	JWTToken   string
}

func NewA1CEClient() *A1CEClient {
	return &A1CEClient{
		BaseURL: "https://a1ce.cmkl.ac.th/api", // Correct A1CE API URL
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		JWTToken: "", // Will be set from request context
	}
}

// GetStudentProfile fetches comprehensive student data from A1CE
func (c *A1CEClient) GetStudentProfile(studentID string) (*StudentProfile, error) {
	profile := &StudentProfile{
		StudentID:           studentID,
		Competencies:        make(map[string]float64),
		CompletedCourses:    []string{},
		DistributionCredits: make(map[string]A1CECredit),
	}

	// Fetch student identity
	identity, err := c.getStudentIdentity(studentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch student identity: %w", err)
	}
	profile.UniversityCode = identity.UniversityCode
	profile.CurriculumVersion = identity.CurriculumVersion

	// Fetch competency cards
	cards, err := c.getStudentCompetencies(studentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch competencies: %w", err)
	}

	for _, card := range cards {
		profile.Competencies[card.CourseCode] = card.Grade
		if card.Status == "Recorded" {
			profile.CompletedCourses = append(profile.CompletedCourses, card.CourseCode)
		}
	}

	// Fetch graduation requirements
	gradStatus, err := c.getGraduationStatus(studentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch graduation status: %w", err)
	}
	profile.RequiredCompetencies = gradStatus.RequiredCompetencies
	profile.DistributionCredits = gradStatus.DistributionCredits
	profile.TotalCredits = gradStatus.TotalCredits

	return profile, nil
}

// GetCourseCatalog fetches available courses for a semester
func (c *A1CEClient) GetCourseCatalog(semester string, curriculumVersion int) (*CourseCatalogResponse, error) {
	// First, get all subdomain IDs
	subdomains, err := c.getSubdomains(curriculumVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subdomains: %w", err)
	}

	catalog := &CourseCatalogResponse{
		Status:            "success",
		Semester:          semester,
		CurriculumVersion: curriculumVersion,
		Courses:           []Course{},
	}

	// Fetch courses for each subdomain
	for _, subdomainID := range subdomains {
		courses, err := c.getCoursesForSubdomain(subdomainID, semester, curriculumVersion)
		if err != nil {
			// Log error but continue with other subdomains
			fmt.Printf("Warning: failed to fetch courses for subdomain %s: %v\n", subdomainID, err)
			continue
		}
		catalog.Courses = append(catalog.Courses, courses...)
	}

	catalog.TotalCourses = len(catalog.Courses)
	return catalog, nil
}

// Helper methods for individual API calls

func (c *A1CEClient) getStudentIdentity(studentID string) (*A1CEStudentIdentity, error) {
	url := fmt.Sprintf("%s/student/identity?student_id=%s", c.BaseURL, studentID)

	var input struct {
		Student A1CEStudentIdentity `json:"student"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	fmt.Println("/student/identity struct: ", input)
	identity := input.Student

	return &identity, nil
}

func (c *A1CEClient) getStudentCompetencies(studentID string) ([]A1CECompetencyCard, error) {
	url := fmt.Sprintf("%s/student/cards?student_id=%s", c.BaseURL, studentID)

	var input struct {
		Info struct {
			Cards []A1CECompetencyCard `json:"cards"`
		} `json:"card_info"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	cards := input.Info.Cards

	return cards, nil
}

func (c *A1CEClient) getGraduationStatus(studentID string) (*A1CEGraduationStatus, error) {
	url := fmt.Sprintf("%s/student/graduation/status?student_id=%s", c.BaseURL, studentID)

	var input struct {
		Status struct {
			RequiredCompetencies []struct {
				Code string `json:"competency_code"`
			} `json:"required_course_not_taken"`
			A1CECreditStatus
		} `json:"graduationstatus"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	var status A1CEGraduationStatus
	for _, competency := range input.Status.RequiredCompetencies {
		status.RequiredCompetencies = append(status.RequiredCompetencies, competency.Code)
	}
	status.A1CECreditStatus = input.Status.A1CECreditStatus

	return &status, nil
}

func (c *A1CEClient) getSubdomains(curriculumVersion int) ([]string, error) {
	url := fmt.Sprintf("%s/subdomain?curriculum_version=%d", c.BaseURL, curriculumVersion)

	var subdomains []string
	if err := c.makeRequest("GET", url, &subdomains); err != nil {
		return nil, err
	}

	return subdomains, nil
}

func (c *A1CEClient) getCoursesForSubdomain(subdomainID, semester string, curriculumVersion int) ([]Course, error) {
	url := fmt.Sprintf("%s/competency?subdomain_id=%s&semester_name=%s&curriculum_version=%d",
		c.BaseURL, subdomainID, semester, curriculumVersion)

	var courses []Course
	if err := c.makeRequest("GET", url, &courses); err != nil {
		return nil, err
	}

	return courses, nil
}

func (c *A1CEClient) getCoursePrerequisites(courseID string) ([]string, error) {
	url := fmt.Sprintf("%s/competency/prerequisite?focus_competency_id=%s", c.BaseURL, courseID)

	var prerequisites []string
	if err := c.makeRequest("GET", url, &prerequisites); err != nil {
		return nil, err
	}

	return prerequisites, nil
}

// Generic HTTP request handler
func (c *A1CEClient) makeRequest(method, url string, result interface{}) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add JWT token if available
	if c.JWTToken != "" {
		req.AddCookie(&http.Cookie{Name: "jwt", Value: c.JWTToken})
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "A1CE-Recommender/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if err := json.Unmarshal(body, result); err != nil {
		fmt.Printf("\tbad body: %s\n", string(body))
		fmt.Printf("\tbad header: %v\n", resp.Header)
		return fmt.Errorf("failed to parse response: %w;", err)
	}

	// dec := json.NewDecoder(resp.Body)
	// if err := dec.Decode(result); err != nil {
	// 	return fmt.Errorf("failed to parse response: %w;", err)
	// }

	return nil
}

// // Mock data for testing when A1CE is unavailable
// func (c *A1CEClient) GetMockStudentProfile(studentID string) *StudentProfile {
// 	return &StudentProfile{
// 		StudentID:         studentID,
// 		UniversityCode:    "CMKL",
// 		CurriculumVersion: 7,
// 		Competencies: map[string]float64{
// 			"AIC-101": 3.5,
// 			"SEN-108": 4.0,
// 			"MAT-201": 2.8,
// 		},
// 		CompletedCourses: []string{"AIC-101", "SEN-108", "MAT-201"},
// 		DistributionCredits: map[string]float64{
// 			"AI":   24,
// 			"SE":   12,
// 			"Math": 12,
// 		},
// 		TotalCredits: 48,
// 		RequiredCompetencies: []string{
// 			"Machine Learning",
// 			"Distributed Systems",
// 		},
// 		MaxCreditLoad: 60,
// 		InterestWeights: map[string]float64{
// 			"AI":   0.6,
// 			"SE":   0.3,
// 			"Math": 0.1,
// 		},
// 	}
// }