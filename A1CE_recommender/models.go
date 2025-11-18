package main

import "time"

// Request structures
type RecommendationRequest struct {
	StudentID     string                 `json:"student_id"`
	Semester      string                 `json:"semester"`
	MaxCreditLoad float64                `json:"max_credit_load"`
	MaxSets       int                    `json:"max_sets"`
	Constraints   *RecommendationFilters `json:"constraints,omitempty"`
}

type RecommendationFilters struct {
	PreferredSubdomains []string `json:"preferred_subdomains,omitempty"`
	ExcludeCourses      []string `json:"exclude_courses,omitempty"`
	TimePreferences     string   `json:"time_preferences,omitempty"`
}

// Student profile structures
type StudentProfile struct {
	StudentID            string                `json:"student_id"`
	UniversityCode       string                `json:"university_code"`
	CurriculumVersion    int                   `json:"curriculum_version"`
	Competencies         map[string]float64    `json:"competencies"`
	CompletedCourses     []string              `json:"completed_courses"`
	DistributionCredits  map[string]A1CECredit `json:"distribution_credits"`
	RequiredCompetencies []string              `json:"required_competencies"`
	TotalCredits         A1CECredit            `json:"total_credits"`
	InterestWeights      map[string]float64    `json:"interest_weights"`
	MaxCreditLoad        float64               `json:"max_credit_load"`
	Semester             string                `json:"semester"` // Currently not used
}

// Course structures
type Course struct {
	CourseID             string             `json:"course_id"`
	CourseName           string             `json:"course_name"`
	Description          string             `json:"description,omitempty"`
	CreditHours          float64            `json:"credit_hours"`
	SubdomainID          string             `json:"subdomain_id"`
	SubdomainName        string             `json:"subdomain_name,omitempty"`
	RequiredCompetencies map[string]float64 `json:"required_competencies"`
	TeachesCompetencies  []string           `json:"teaches_competencies"`
	Prerequisites        []string           `json:"prerequisites"`
	SemesterOffered      string             `json:"semester_offered"`
}

// Curriculum requirements
type CurriculumRequirements struct {
	CurriculumVersion        int                `json:"curriculum_version"`
	RequiredCompetencies     []string           `json:"required_competencies"`
	DistributionRequirements map[string]float64 `json:"distribution_requirements"`
	TotalCreditsRequired     float64            `json:"total_credits_required"`
}

// Recommendation output structures
type RecommendedCourse struct {
	Course                 Course   `json:"course"`
	FitScore               float64  `json:"fit_score"`
	MatchedCompetencies    []string `json:"matched_competencies"`
	MissingCompetencies    []string `json:"missing_competencies"`
	CompetencyMatchScore   float64  `json:"competency_match_score"`
	InterestAlignmentScore float64  `json:"interest_alignment_score"`
	ProgramProgressScore   float64  `json:"program_progress_score"`
	Reason                 string   `json:"reason"`
}

type RecommendationSet struct {
	StudentID            string                 `json:"student_id"`
	Semester             string                 `json:"semester"`
	RecommendedSet       []RecommendedCourse    `json:"recommended_set"`
	TotalCredits         float64                `json:"total_credits"`
	Metrics              EvaluationMetrics      `json:"metrics"`
	DistributionCoverage map[string]float64     `json:"distribution_coverage"`
	Metadata             RecommendationMetadata `json:"metadata"`
	Status               string                 `json:"status"`
}

type EvaluationMetrics struct {
	GoodnessScore           float64 `json:"goodness_score"`
	SkillCoveragePercentage float64 `json:"skill_coverage_percentage"`
	PrerequisiteCompliance  float64 `json:"prerequisite_compliance_percentage"`
	ProgramProgressFit      float64 `json:"program_progress_fit"`
}

type RecommendationMetadata struct {
	GenerationTimestamp time.Time `json:"generation_timestamp"`
	AlgorithmVersion    string    `json:"algorithm_version"`
	ProcessingTimeMs    int64     `json:"processing_time_ms"`
}

// A1CE API response structures
type A1CEStudentIdentity struct {
	StudentID         string `json:"id"`
	UniversityCode    string `json:"university_code"`
	CurriculumVersion int    `json:"curriculum_version"`
}

type A1CECompetencyCard struct {
	CompetencyID string  `json:"id"`
	CourseCode   string  `json:"competency_code"`
	Grade        float64 `json:"mastery_level"`
	Status       string  `json:"status"`
}

// for A1CECreditStatus
type A1CECredit struct {
	Earned   int `json:"total_earned_credits"`
	Required int `json:"total_required_credits"`
	Working  int `json:"total_working_credits"`
}

type A1CECreditStatus struct {
	DistributionCredits map[string]A1CECredit `json:"distribution_area_credit"`
	TotalCredits        A1CECredit            `json:"overall_credit"`
}

type A1CEGraduationStatus struct {
	RequiredCompetencies []string `json:"required_course_not_taken"`
	A1CECreditStatus
}

type CourseCatalogResponse struct {
	Status            string   `json:"status"`
	Semester          string   `json:"semester"`
	CurriculumVersion int      `json:"curriculum_version"`
	Courses           []Course `json:"courses"`
	TotalCourses      int      `json:"total_courses"`
}
