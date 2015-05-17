package yetanothersmtpd

import "fmt"

// StatusCode represents SMTP status code
type StatusCode int

// SMTP Status codes
const (
	StatusSuccess              StatusCode = 200
	StatusSystem               StatusCode = 211
	StatusHelpMessage          StatusCode = 214
	StatusServiceReady         StatusCode = 220
	StatusServiceClosing       StatusCode = 221
	StatusAuthenticated        StatusCode = 235
	StatusOK                   StatusCode = 250
	StatusNotLocalWillForward  StatusCode = 251
	StatusCantVerifyWillAccept StatusCode = 252

	StatusProvideCredentials StatusCode = 334
	StatusStartMailInput     StatusCode = 354

	StatusServiceNotAvailable           StatusCode = 421
	StatusMailboxTemporarilyUnavailable StatusCode = 450
	StatusLocalError                    StatusCode = 451
	StatusInsufficientStorage           StatusCode = 452

	StatusCommandUnrecognized           StatusCode = 500
	StatusSyntaxError                   StatusCode = 501
	StatusCommandNotImplemented         StatusCode = 502
	StatusBadSequence                   StatusCode = 503
	StatusParameterNotImplemented       StatusCode = 504
	StatusDoesNotAcceptMail             StatusCode = 521
	StatusAccessDenied                  StatusCode = 530
	StatusMailboxPermanentlyUnavailable StatusCode = 550
	StatusUserNotLocal                  StatusCode = 551
	StatusExceededStorageAllocation     StatusCode = 552
	StatusMailboxNameNotAllowed         StatusCode = 553
	StatusTransactionFailed             StatusCode = 554
)

var (
	AuthSuccess = NewReportableStatus(StatusAuthenticated, "OK, you are now authenticated")
	GoAhead     = NewReportableStatus(StatusOK, "Go ahead")
	ThankYou    = NewReportableStatus(StatusOK, "Thank you")

	ErrBadSequence         = NewReportableStatus(StatusBadSequence, "Invalid command sequence")
	ErrDecodingCredentials = NewReportableStatus(StatusSyntaxError, "Couldn't decode your credentials")
	ErrInvalidSyntax       = NewReportableStatus(StatusSyntaxError, "Invalid syntax")
	ErrMalformedEmail      = NewReportableStatus(StatusSyntaxError, "Malformed email address")
	ErrNoHelo              = NewReportableStatus(StatusBadSequence, "Please introduce yourself first")
)

// ReportableStatus is a trivial implementation of 'error' interface. It does
// not necessarily mean an Error though, but allows to differentiate between
// reportable and non-reportable events. Some of the former might just as well
// be success events.
type ReportableStatus struct {
	Code    StatusCode
	Message string
}

// NewReportableStatus provides a helper function for creating instances of
// NewReportableStatus.
func NewReportableStatus(code StatusCode, format string, args ...interface{}) error {
	return &ReportableStatus{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

func (r *ReportableStatus) Error() string {
	return fmt.Sprintf("%d %s", r.Code, r.Message)
}
