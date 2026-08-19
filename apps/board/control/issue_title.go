package control

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const IssueTitleMaxLength = 120

func normalizeIssueTitle(value string) (string, error) {
	title := strings.TrimSpace(value)
	if title == "" {
		return "", errors.New("Issue 标题不能为空")
	}
	if utf8.RuneCountInString(title) > IssueTitleMaxLength {
		return "", fmt.Errorf("Issue 标题不能超过 %d 个字符", IssueTitleMaxLength)
	}
	return title, nil
}
