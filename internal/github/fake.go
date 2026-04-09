package github

import "time"

// FakeClient implements NotificationAPI for testing. It returns preconfigured
// data and records calls for assertions.
type FakeClient struct {
	Notifications []Notification
	Details       map[string]*ThreadDetail // keyed by subjectURL
	CurrentUser   string

	// Recorded calls
	MarkedRead    []string
	AllMarkedRead bool
	MutedThreads  []string
	Unsubscribed  []string

	// Error injection
	ListErr   error
	DetailErr error
}

var _ NotificationAPI = (*FakeClient)(nil)

func (f *FakeClient) ListNotifications(_ ListOptions) ([]Notification, error) {
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	return f.Notifications, nil
}

func (f *FakeClient) MarkThreadRead(threadID string) error {
	f.MarkedRead = append(f.MarkedRead, threadID)
	return nil
}

func (f *FakeClient) MarkAllRead(_ *time.Time) error {
	f.AllMarkedRead = true
	return nil
}

func (f *FakeClient) MuteThread(threadID string) error {
	f.MutedThreads = append(f.MutedThreads, threadID)
	return nil
}

func (f *FakeClient) UnsubscribeThread(threadID string) error {
	f.Unsubscribed = append(f.Unsubscribed, threadID)
	return nil
}

func (f *FakeClient) FetchThreadDetail(subjectURL, _ string) (*ThreadDetail, error) {
	if f.DetailErr != nil {
		return nil, f.DetailErr
	}
	if f.Details != nil {
		if d, ok := f.Details[subjectURL]; ok {
			return d, nil
		}
	}
	return &ThreadDetail{}, nil
}

func (f *FakeClient) GetCurrentUser() (string, error) {
	return f.CurrentUser, nil
}
