package semver

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	numerals     string = "0123456789"
	alphabet            = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	alphaNumeric        = alphabet + numerals
	SemVersion          = "0.2.1"
)

var (
	helpFlag    bool
	versionFlag bool
)

// Version struct represents a semantic version
type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
}

func (v *Version) String() string {
	b := make([]byte, 0, 5)
	b = strconv.AppendUint(b, v.Major, 10)
	b = append(b, '.')
	b = strconv.AppendUint(b, v.Minor, 10)
	b = append(b, '.')
	b = strconv.AppendUint(b, v.Patch, 10)

	return string(b)
}

// Equal accepts to semver strings and compares them
func (v *Version) Equal(v2 *Version) bool {
	if v.Major != v2.Major {
		return false
	}

	if v.Minor != v2.Minor {
		return false
	}

	if v.Patch != v2.Patch {
		return false
	}

	return true
}

// CheckFlag takes a semver Version struct and sets
// a -version flag to return a semver string
func (v *Version) CheckFlag() {
	if versionFlag {
		fmt.Println(v.String())
		os.Exit(0)
	}

	// // Only continue if the main package hasn't parsed flags
	// if flag.Parsed() {
	// 	return
	// }
	//
	// if helpFlag {
	// 	fmt.Println("Usage of version:")
	// 	flag.PrintDefaults()
	// 	// os.Exit(0)
	// }
}

// SetVersion takes a semver string and creates a version flag
// It will parse all flags (versionFS.Parse())
func SetVersion(s string) error {
	// Create Version struct for supplied strings
	v, err := NewVersion(s)
	if err != nil {
		return err
	}

	// Tag git repo if one exist

	// Create version flag
	v.CheckFlag()

	return nil
}

// Equal accepts to semver strings and compares them
func Equal(s1 string, s2 string) bool {
	// Create Version struct for supplied strings
	v, err := NewVersion(s1)
	if err != nil {
		return false
	}

	v2, err := NewVersion(s2)
	if err != nil {
		return false
	}

	return v.Equal(v2)
}

// NewVersion parses a version string
func NewVersion(s string) (*Version, error) {
	if len(s) == 0 {
		return nil, errors.New("Version string empty")
	}

	// Split into major.minor.patch
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil, errors.New("Major.Minor.Patch elements not found")
	}

	// Major
	if !containsOnly(parts[0], numerals) {
		return nil, fmt.Errorf("Invalid character(s) found in major number %s", parts[0])
	}
	if hasLeadingZeroes(parts[0]) {
		return nil, fmt.Errorf("Major number must not contain leading zeroes %s", parts[0])
	}
	major, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}

	// Minor
	if !containsOnly(parts[1], numerals) {
		return nil, fmt.Errorf("Invalid character(s) found in minor number %s", parts[1])
	}
	if hasLeadingZeroes(parts[1]) {
		return nil, fmt.Errorf("Minor number must not contain leading zeroes %s", parts[1])
	}
	minor, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}

	// Patch
	if !containsOnly(parts[2], numerals) {
		return nil, fmt.Errorf("Invalid character(s) found in patch number %s", parts[2])
	}
	if hasLeadingZeroes(parts[2]) {
		return nil, fmt.Errorf("Patch number must not contain leading zeroes %s", parts[2])
	}
	patch, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return nil, err
	}

	v := new(Version)
	v.Major = major
	v.Minor = minor
	v.Patch = patch

	return v, nil
}

func containsOnly(s string, compare string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune(compare, r)
	}) == -1
}

func hasLeadingZeroes(s string) bool {
	return len(s) > 1 && s[0] == '0'
}

// init creates -version flag or checks for version arg
func init() {
	// Setup version flag
	var versionUsage = "Prints current version"
	var versionPtr = flag.Bool("version", false, versionUsage)

	// Set up short hand flags
	flag.BoolVar(versionPtr, "v", false, versionUsage+" (shorthand)")

	// Only continue if an arg is passed
	if len(os.Args) <= 1 {
		return
	}

	switch os.Args[1] {
	case "--version":
		fallthrough
	case "-version":
		fallthrough
	case "--v":
		fallthrough
	case "-v":
		versionFlag = true
		return
	}

	// switch os.Args[1] {
	// case "--help":
	// 	fallthrough
	// case "-help":
	// 	fallthrough
	// case "--h":
	// 	fallthrough
	// case "-h":
	// 	helpFlag = true
	// 	return
	// }
}
