package test

// stringPtr helper function for creating string pointers
// Shared across all test files to avoid duplication
func stringPtr(s string) *string {
	return &s
}