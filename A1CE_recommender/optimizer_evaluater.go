package main

import "math"

// OptimizeCourseSet selects optimal combination of courses for the semester
func OptimizeCourseSet(
	scoredCourses []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
	maxCreditLoad float64,
) []RecommendedCourse {
	var selectedCourses []RecommendedCourse
	totalCredits := 0.0
	coveredCompetencies := getMapKeys(studentProfile.Competencies)
	distributionCoverage := make(map[string]float64)

	// Copy existing distribution credits
	for k, v := range studentProfile.DistributionCredits {
		distributionCoverage[k] = float64(v.Earned)
	}

	minCredits := 36.0
	targetCredits := math.Min(maxCreditLoad, 60.0)
	subdomainCount := make(map[string]int)

	for _, courseRec := range scoredCourses {
		course := courseRec.Course

		// Constraint 1: Credit limit
		if totalCredits+course.CreditHours > targetCredits {
			if totalCredits >= minCredits {
				continue
			} else if totalCredits+course.CreditHours > maxCreditLoad {
				continue
			}
		}

		// Constraint 2: Diversity - avoid too many courses from same subdomain
		subdomain := course.SubdomainID
		if subdomainCount[subdomain] >= 3 {
			continue
		}

		// Calculate marginal value of adding this course
		newComps := difference(course.TeachesCompetencies, coveredCompetencies)
		newCompValue := float64(len(newComps))

		// Distribution area progress
		requiredInSubdomain := 0.0
		if req, exists := requirements.DistributionRequirements[subdomain]; exists {
			requiredInSubdomain = req
		}

		currentInSubdomain := 0.0
		if current, exists := distributionCoverage[subdomain]; exists {
			currentInSubdomain = current
		}

		gapInSubdomain := math.Max(0, requiredInSubdomain-currentInSubdomain)
		distributionValue := 0.0
		if gapInSubdomain > 0 {
			distributionValue = math.Min(1.0, course.CreditHours/gapInSubdomain)
		} else {
			distributionValue = 0.2
		}

		// Combined marginal value
		marginalValue := 0.3*(newCompValue/5.0) +
			0.3*distributionValue +
			0.4*courseRec.FitScore

		// Accept if marginal value is high enough
		acceptanceThreshold := 0.5 * (1.0 - totalCredits/targetCredits)
		if marginalValue >= acceptanceThreshold {
			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			coveredCompetencies = append(coveredCompetencies, course.TeachesCompetencies...)
			distributionCoverage[subdomain] += course.CreditHours
			subdomainCount[subdomain]++

			if totalCredits >= targetCredits {
				break
			}
		}
	}

	// If we didn't reach minimum credits, add more courses
	if totalCredits < minCredits {
		for _, courseRec := range scoredCourses {
			if containsRecommendedCourse(selectedCourses, courseRec) {
				continue
			}
			course := courseRec.Course
			if totalCredits+course.CreditHours <= maxCreditLoad {
				selectedCourses = append(selectedCourses, courseRec)
				totalCredits += course.CreditHours
				if totalCredits >= minCredits {
					break
				}
			}
		}
	}

	return selectedCourses
}

// EvaluateRecommendationSet calculates quality metrics
func EvaluateRecommendationSet(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
) *EvaluationMetrics {
	skillCoverage := CalculateSkillCoverage(recommendedSet, studentProfile, requirements)
	prereqCompliance := CalculatePrerequisiteCompliance(recommendedSet, studentProfile)
	programProgressFit := CalculateProgramProgressFit(recommendedSet, studentProfile, requirements)

	goodnessScore := 0.3*skillCoverage +
		0.3*prereqCompliance +
		0.4*programProgressFit

	return &EvaluationMetrics{
		GoodnessScore:           goodnessScore,
		SkillCoveragePercentage: skillCoverage,
		PrerequisiteCompliance:  prereqCompliance,
		ProgramProgressFit:      programProgressFit,
	}
}

// CalculateSkillCoverage measures percentage of skill gaps addressed
func CalculateSkillCoverage(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
) float64 {
	requiredComps := requirements.RequiredCompetencies
	currentComps := getMapKeys(studentProfile.Competencies)
	missingComps := difference(requiredComps, currentComps)

	if len(missingComps) == 0 {
		return 1.0
	}

	var coveredByRecommendations []string
	for _, courseRec := range recommendedSet {
		taughtComps := courseRec.Course.TeachesCompetencies
		coveredByRecommendations = append(coveredByRecommendations, taughtComps...)
	}

	addressedGaps := intersection(missingComps, coveredByRecommendations)
	return float64(len(addressedGaps)) / float64(len(missingComps))
}

// CalculatePrerequisiteCompliance verifies all prerequisites are satisfied
func CalculatePrerequisiteCompliance(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
) float64 {
	if len(recommendedSet) == 0 {
		return 1.0
	}

	compliantCourses := 0
	for _, courseRec := range recommendedSet {
		if CheckPrerequisites(courseRec.Course, studentProfile) {
			compliantCourses++
		}
	}

	return float64(compliantCourses) / float64(len(recommendedSet))
}

// CalculateProgramProgressFit measures degree completion advancement
func CalculateProgramProgressFit(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
) float64 {
	// Component 1: Required Competency Progress
	requiredComps := requirements.RequiredCompetencies
	currentComps := getMapKeys(studentProfile.Competencies)
	missingRequired := difference(requiredComps, currentComps)

	competencyProgress := 1.0
	if len(missingRequired) > 0 {
		var covered []string
		for _, courseRec := range recommendedSet {
			taught := courseRec.Course.TeachesCompetencies
			covered = append(covered, intersection(taught, missingRequired)...)
		}
		covered = unique(covered)
		competencyProgress = float64(len(covered)) / float64(len(missingRequired))
	}

	// Component 2: Distribution Area Progress
	var distributionProgressScores []float64

	distributionCoverage := make(map[string]float64)
	for _, courseRec := range recommendedSet {
		subdomain := courseRec.Course.SubdomainID
		distributionCoverage[subdomain] += courseRec.Course.CreditHours
	}

	for subdomain, requiredCredits := range requirements.DistributionRequirements {
		currentCredits := 0.0
		if credits, exists := studentProfile.DistributionCredits[subdomain]; exists {
			currentCredits = float64(credits.Earned)
		}

		recommendedCredits := 0.0
		if credits, exists := distributionCoverage[subdomain]; exists {
			recommendedCredits = credits
		}

		remainingGap := math.Max(0, requiredCredits-currentCredits)
		areaProgress := 1.0
		if remainingGap > 0 {
			areaProgress = math.Min(1.0, recommendedCredits/remainingGap)
		}
		distributionProgressScores = append(distributionProgressScores, areaProgress)
	}

	distributionProgress := 1.0
	if len(distributionProgressScores) > 0 {
		distributionProgress = mean(distributionProgressScores)
	}

	// Weighted combination
	return 0.6*competencyProgress + 0.4*distributionProgress
}

// Helper functions
func containsRecommendedCourse(courses []RecommendedCourse, course RecommendedCourse) bool {
	for _, c := range courses {
		if c.Course.CourseID == course.Course.CourseID {
			return true
		}
	}
	return false
}

func unique(slice []string) []string {
	keys := make(map[string]bool)
	var result []string
	for _, entry := range slice {
		if _, exists := keys[entry]; !exists {
			keys[entry] = true
			result = append(result, entry)
		}
	}
	return result
}
