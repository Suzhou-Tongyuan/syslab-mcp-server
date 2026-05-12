package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDirName   = "syslab-skills"
	defaultSkillFile = "SKILL.md"
	maxSkillChars    = 6000
)

func ResolveRoot(explicitRoot string) (string, error) {
	if strings.TrimSpace(explicitRoot) != "" {
		return filepath.Abs(explicitRoot)
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path for skills: %w", err)
	}

	serverDir := filepath.Dir(exePath)
	toolsDir := filepath.Dir(serverDir)
	return filepath.Abs(filepath.Join(toolsDir, defaultDirName))
}

func ResolvePrimarySkillFile(explicitFile string, explicitRoot string) (string, error) {
	if strings.TrimSpace(explicitFile) != "" {
		return filepath.Abs(explicitFile)
	}

	root, err := ResolveRoot(explicitRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, defaultSkillFile), nil
}

func LoadSkillFile(path string) (string, string, bool, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", false, nil
	}

	skillPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", false, err
	}

	data, err := os.ReadFile(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", skillPath, false, nil
		}
		return "", skillPath, false, fmt.Errorf("read skill file %s: %w", skillPath, err)
	}

	content, truncated := normalizeSkillContent(string(data))
	if content == "" {
		return "", skillPath, truncated, nil
	}
	return content, skillPath, truncated, nil
}

func LoadPrimarySkill(root string) (string, string, error) {
	skillPath, err := ResolvePrimarySkillFile("", root)
	if err != nil {
		return "", "", err
	}

	content, resolvedPath, _, err := LoadSkillFile(skillPath)
	return content, resolvedPath, err
}

func normalizeSkillContent(content string) (string, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	trimmed := make([]string, 0, len(lines))
	lastBlank := false

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		isBlank := strings.TrimSpace(line) == ""
		if isBlank {
			if lastBlank {
				continue
			}
			lastBlank = true
		} else {
			lastBlank = false
		}
		trimmed = append(trimmed, line)
	}

	content = strings.TrimSpace(strings.Join(trimmed, "\n"))
	truncated := false
	if len(content) > maxSkillChars {
		content = strings.TrimSpace(content[:maxSkillChars]) + "\n...[truncated]"
		truncated = true
	}
	return content, truncated
}
