package nodedist

import (
	"encoding/json"
	"fmt"
	"os"
)

const scheduleURL = "https://raw.githubusercontent.com/nodejs/Release/main/schedule.json"

type ReleaseSchedule struct {
	Start       string `json:"start"`
	LTS         string `json:"lts"`
	Maintenance string `json:"maintenance"`
	End         string `json:"end"`
	Codename    string `json:"codename"`
}

type Schedule map[string]ReleaseSchedule

func FetchSchedule(cachePath string) (Schedule, error) {
	data, err := fetchURL(scheduleURL)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(cachePath, data, 0o644)
	return parseSchedule(data)
}

func LoadSchedule(cachePath string) (Schedule, error) {
	data, err := cachedBytes(cachePath, scheduleURL)
	if err != nil {
		return nil, err
	}
	return parseSchedule(data)
}

func parseSchedule(data []byte) (Schedule, error) {
	var schedule Schedule
	if err := json.Unmarshal(data, &schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s Schedule) EndOfLife(version string) string {
	entry, ok := s[scheduleKey(version)]
	if !ok {
		return ""
	}
	return entry.End
}

func scheduleKey(version string) string {
	parsed, err := parseVersion(version)
	if err != nil {
		return ""
	}
	if parsed.major == 0 {
		return fmt.Sprintf("v0.%d", parsed.minor)
	}
	return fmt.Sprintf("v%d", parsed.major)
}
