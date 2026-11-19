package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyIQCustomQuestions = "iq_test_custom_questions"

	CustomQuestionKind     = "custom"
	CustomHTMLQuestionKind = "custom_html"

	IQCustomQuestionOutputText = "text"
	IQCustomQuestionOutputHTML = "html"

	IQCustomQuestionsMaxCount   = 100
	IQCustomQuestionMaxTitle    = 100
	IQCustomQuestionMaxPrompt   = 32000
	IQCustomQuestionMaxContract = 4000
	IQCustomQuestionMaxNote     = 2000
)

// IQCustomQuestion is a reusable user-defined intelligence test question stored
// in system settings so admins and observers can run and schedule custom test methods.
type IQCustomQuestion struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Prompt           string `json:"prompt"`
	OutputType       string `json:"output_type"`
	DeliveryContract string `json:"delivery_contract,omitempty"`
	ReferenceNote    string `json:"reference_note,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// IQCustomQuestionInput is the validated payload for creating or updating a custom question.
type IQCustomQuestionInput struct {
	Title            string `json:"title"`
	Prompt           string `json:"prompt"`
	OutputType       string `json:"output_type"`
	DeliveryContract string `json:"delivery_contract"`
	ReferenceNote    string `json:"reference_note"`
}

var errIQCustomQuestionsUnavailable = infraerrors.ServiceUnavailable("IQ_CUSTOM_QUESTIONS_UNAVAILABLE", "custom question storage unavailable")

func normalizeIQCustomQuestionInput(input IQCustomQuestionInput) (IQCustomQuestionInput, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", "question title is required")
	}
	if utf8.RuneCountInString(title) > IQCustomQuestionMaxTitle {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", fmt.Sprintf("question title must be at most %d characters", IQCustomQuestionMaxTitle))
	}

	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", "question prompt is required")
	}
	if len(prompt) > IQCustomQuestionMaxPrompt {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", fmt.Sprintf("question prompt must be at most %d bytes", IQCustomQuestionMaxPrompt))
	}

	outputType := strings.ToLower(strings.TrimSpace(input.OutputType))
	if outputType == "" {
		outputType = IQCustomQuestionOutputText
	}
	if outputType != IQCustomQuestionOutputText && outputType != IQCustomQuestionOutputHTML {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", "output_type must be text or html")
	}

	contract := strings.TrimSpace(input.DeliveryContract)
	if len(contract) > IQCustomQuestionMaxContract {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", fmt.Sprintf("delivery contract must be at most %d bytes", IQCustomQuestionMaxContract))
	}

	note := strings.TrimSpace(input.ReferenceNote)
	if len(note) > IQCustomQuestionMaxNote {
		return IQCustomQuestionInput{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", fmt.Sprintf("reference note must be at most %d bytes", IQCustomQuestionMaxNote))
	}

	return IQCustomQuestionInput{
		Title:            title,
		Prompt:           prompt,
		OutputType:       outputType,
		DeliveryContract: contract,
		ReferenceNote:    note,
	}, nil
}

func generateIQCustomQuestionID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate custom question id: %w", err)
	}
	return "cq_" + hex.EncodeToString(raw[:]), nil
}

func (s *SettingService) loadIQCustomQuestionsLocked(ctx context.Context) ([]IQCustomQuestion, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errIQCustomQuestionsUnavailable
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyIQCustomQuestions)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []IQCustomQuestion{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []IQCustomQuestion{}, nil
	}
	var items []IQCustomQuestion
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid custom questions JSON: %w", err)
	}
	if items == nil {
		return []IQCustomQuestion{}, nil
	}
	for i := range items {
		if items[i].OutputType != IQCustomQuestionOutputHTML {
			items[i].OutputType = IQCustomQuestionOutputText
		}
	}
	return items, nil
}

func (s *SettingService) saveIQCustomQuestionsLocked(ctx context.Context, items []IQCustomQuestion) error {
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, SettingKeyIQCustomQuestions, string(raw))
}

// ListIQCustomQuestions returns all saved custom intelligence test questions.
func (s *SettingService) ListIQCustomQuestions(ctx context.Context) ([]IQCustomQuestion, error) {
	if s == nil || s.settingRepo == nil {
		return []IQCustomQuestion{}, nil
	}
	s.iqCustomQuestionsMu.Lock()
	defer s.iqCustomQuestionsMu.Unlock()
	return s.loadIQCustomQuestionsLocked(ctx)
}

// CreateIQCustomQuestion validates and appends a new custom intelligence test question.
func (s *SettingService) CreateIQCustomQuestion(ctx context.Context, input IQCustomQuestionInput) (IQCustomQuestion, error) {
	if s == nil || s.settingRepo == nil {
		return IQCustomQuestion{}, errIQCustomQuestionsUnavailable
	}
	normalized, err := normalizeIQCustomQuestionInput(input)
	if err != nil {
		return IQCustomQuestion{}, err
	}
	s.iqCustomQuestionsMu.Lock()
	defer s.iqCustomQuestionsMu.Unlock()

	items, err := s.loadIQCustomQuestionsLocked(ctx)
	if err != nil {
		return IQCustomQuestion{}, err
	}
	if len(items) >= IQCustomQuestionsMaxCount {
		return IQCustomQuestion{}, infraerrors.BadRequest("CUSTOM_QUESTION_LIMIT", fmt.Sprintf("at most %d custom questions are supported", IQCustomQuestionsMaxCount))
	}
	id, err := generateIQCustomQuestionID()
	if err != nil {
		return IQCustomQuestion{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	created := IQCustomQuestion{
		ID:               id,
		Title:            normalized.Title,
		Prompt:           normalized.Prompt,
		OutputType:       normalized.OutputType,
		DeliveryContract: normalized.DeliveryContract,
		ReferenceNote:    normalized.ReferenceNote,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	items = append(items, created)
	if err := s.saveIQCustomQuestionsLocked(ctx, items); err != nil {
		return IQCustomQuestion{}, err
	}
	return created, nil
}

// UpdateIQCustomQuestion validates and updates an existing custom intelligence test question by ID.
func (s *SettingService) UpdateIQCustomQuestion(ctx context.Context, id string, input IQCustomQuestionInput) (IQCustomQuestion, error) {
	if s == nil || s.settingRepo == nil {
		return IQCustomQuestion{}, errIQCustomQuestionsUnavailable
	}
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return IQCustomQuestion{}, infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", "question id is required")
	}
	normalized, err := normalizeIQCustomQuestionInput(input)
	if err != nil {
		return IQCustomQuestion{}, err
	}
	s.iqCustomQuestionsMu.Lock()
	defer s.iqCustomQuestionsMu.Unlock()

	items, err := s.loadIQCustomQuestionsLocked(ctx)
	if err != nil {
		return IQCustomQuestion{}, err
	}
	for i := range items {
		if items[i].ID == trimmedID {
			items[i].Title = normalized.Title
			items[i].Prompt = normalized.Prompt
			items[i].OutputType = normalized.OutputType
			items[i].DeliveryContract = normalized.DeliveryContract
			items[i].ReferenceNote = normalized.ReferenceNote
			items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := s.saveIQCustomQuestionsLocked(ctx, items); err != nil {
				return IQCustomQuestion{}, err
			}
			return items[i], nil
		}
	}
	return IQCustomQuestion{}, infraerrors.NotFound("CUSTOM_QUESTION_NOT_FOUND", "custom question not found")
}

// DeleteIQCustomQuestion removes a custom intelligence test question by ID.
func (s *SettingService) DeleteIQCustomQuestion(ctx context.Context, id string) error {
	if s == nil || s.settingRepo == nil {
		return errIQCustomQuestionsUnavailable
	}
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return infraerrors.BadRequest("INVALID_CUSTOM_QUESTION", "question id is required")
	}
	s.iqCustomQuestionsMu.Lock()
	defer s.iqCustomQuestionsMu.Unlock()

	items, err := s.loadIQCustomQuestionsLocked(ctx)
	if err != nil {
		return err
	}
	idx := -1
	for i := range items {
		if items[i].ID == trimmedID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return infraerrors.NotFound("CUSTOM_QUESTION_NOT_FOUND", "custom question not found")
	}
	items = append(items[:idx], items[idx+1:]...)
	return s.saveIQCustomQuestionsLocked(ctx, items)
}
