package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type A1CEClient struct {
	BaseURL        string
	HTTPClient     *http.Client
	JWTToken       string
	UniversityCode string
}

func NewA1CEClient() *A1CEClient {
	return &A1CEClient{
		BaseURL:    "https://a1ce.cmkl.ac.th/api",
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *A1CEClient) GetStudentProfile(studentID string) (*StudentProfile, error) {
	profile := &StudentProfile{
		StudentID:           studentID,
		Competencies:        make(map[string]float64),
		CourseSemesters:     make(map[string]string),
		CompletedCourses:    []string{},
		DistributionCredits: make(map[string]A1CECredit),
	}

	identity, err := c.getStudentIdentity(studentID)
	if err != nil {
		return nil, err
	}

	profile.UniversityCode = identity.UniversityCode
	c.UniversityCode = identity.UniversityCode
	profile.CurriculumVersion = identity.CurriculumVersion

	cards, err := c.getStudentCompetencies(studentID)
	if err == nil {
		for _, card := range cards {
			profile.Competencies[card.CourseCode] = card.Grade
			if card.Semester != "" {
				profile.CourseSemesters[card.CourseCode] = card.Semester
			}
			// CRITICAL: Add to completed list if status is 'Recorded'
			// You might also want to check if grade > 0 or != "W" (Withdrawn)
			if card.Status == "Recorded" {
				profile.CompletedCourses = append(profile.CompletedCourses, card.CourseCode)
			}
		}
	}

	gradStatus, err := c.getGraduationStatus(studentID)
	if err == nil {
		profile.RequiredCompetencies = gradStatus.RequiredCompetencies
		profile.DistributionCredits = gradStatus.DistributionCredits
		profile.TotalCredits = gradStatus.TotalCredits
	}

	return profile, nil
}

func (c *A1CEClient) GetSemesterCompetencies(studentID, semester string) ([]A1CECompetencyCard, error) {
	safeSemester := url.QueryEscape(semester)
	// Fallback manual replace if url.QueryEscape uses + for space which some APIs dislike
	if strings.Contains(semester, " ") && !strings.Contains(safeSemester, "%20") {
		safeSemester = strings.ReplaceAll(semester, " ", "%20")
	}

	url := fmt.Sprintf("%s/student/cards/semester?student_id=%s&semester_name=%s", c.BaseURL, studentID, safeSemester)

	var input struct {
		Info struct {
			Cards []A1CECompetencyCard `json:"cards"`
		} `json:"card_info"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	return input.Info.Cards, nil
}

func (c *A1CEClient) GetCourseCatalog(semester string, curriculumVersion int) (*CourseCatalogResponse, error) {
	subdomains, err := c.getSubdomains(curriculumVersion)
	if err != nil {
		return nil, err
	}

	catalog := &CourseCatalogResponse{
		Status: "success", Semester: semester, CurriculumVersion: curriculumVersion, Courses: []Course{},
	}

	for _, subdomainID := range subdomains {
		courses, err := c.getCoursesForSubdomain(subdomainID, semester, curriculumVersion)
		if err != nil {
			fmt.Printf("Warning: Failed to fetch subdomain %s: %v\n", subdomainID, err)
			continue
		}
		catalog.Courses = append(catalog.Courses, courses...)
	}

	catalog.TotalCourses = len(catalog.Courses)
	return catalog, nil
}

// --- API CALLS ---

func (c *A1CEClient) getStudentIdentity(studentID string) (*A1CEStudentIdentity, error) {
	url := fmt.Sprintf("%s/student/identity?student_id=%s", c.BaseURL, studentID)
	var input struct {
		Student A1CEStudentIdentity `json:"student"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	return &input.Student, nil
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
	return input.Info.Cards, nil
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
	for _, c := range input.Status.RequiredCompetencies {
		status.RequiredCompetencies = append(status.RequiredCompetencies, c.Code)
	}
	status.A1CECreditStatus = input.Status.A1CECreditStatus
	return &status, nil
}

func (c *A1CEClient) getSubdomains(curriculumVersion int) ([]string, error) {
	url := fmt.Sprintf("%s/subdomain?curriculum_version=%d", c.BaseURL, curriculumVersion)
	if c.UniversityCode != "" {
		url += "&university_code=" + c.UniversityCode
	}

	var response struct {
		Pillars []struct {
			Subdomains []struct {
				ID string `json:"id"`
			} `json:"subdomains"`
		} `json:"pillars"`
	}
	if err := c.makeRequest("GET", url, &response); err != nil {
		return nil, err
	}

	var subdomains []string
	for _, p := range response.Pillars {
		for _, s := range p.Subdomains {
			if s.ID != "" {
				subdomains = append(subdomains, s.ID)
			}
		}
	}
	return subdomains, nil
}

func (c *A1CEClient) getCoursesForSubdomain(subdomainID, semester string, curriculumVersion int) ([]Course, error) {
	safeSemester := url.QueryEscape(semester)
	if strings.Contains(semester, " ") && !strings.Contains(safeSemester, "%20") {
		safeSemester = strings.ReplaceAll(semester, " ", "%20")
	}

	url := fmt.Sprintf("%s/competency?subdomain_id=%s&semester_name=%s&curriculum_version=%d",
		c.BaseURL, subdomainID, safeSemester, curriculumVersion)

	if c.UniversityCode != "" {
		url += "&university_code=" + c.UniversityCode
	}

	type APICourse struct {
		ID          string  `json:"id"`
		Code        string  `json:"competency_code"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Credits     float64 `json:"credits"`
	}
	var response struct {
		Competencies []APICourse `json:"competencies"`
	}
	if err := c.makeRequest("GET", url, &response); err != nil {
		return nil, err
	}

	var courses []Course
	for _, ac := range response.Competencies {
		finalID := ac.ID
		if finalID == "" {
			finalID = ac.Code
		}

		courses = append(courses, Course{
			CourseID:             finalID,
			CourseCode:           ac.Code,
			CourseName:           ac.Title,
			Description:          ac.Description,
			CreditHours:          ac.Credits,
			SubdomainID:          subdomainID,
			RequiredCompetencies: make(map[string]float64),
			TeachesCompetencies:  []string{},
			Prerequisites:        []string{},
		})
	}
	return courses, nil
}

func (c *A1CEClient) makeRequest(method, url string, result interface{}) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	if c.JWTToken != "" {
		req.AddCookie(&http.Cookie{Name: "jwt", Value: c.JWTToken})
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return json.Unmarshal(body, result)
}
