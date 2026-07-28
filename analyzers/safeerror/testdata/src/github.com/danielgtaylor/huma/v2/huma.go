// Stub for testing the safeerror analyzer.
package huma

type StatusError interface {
	error
	GetStatus() int
}

type statusError struct {
	status  int
	message string
}

func (e *statusError) Error() string  { return e.message }
func (e *statusError) GetStatus() int { return e.status }

func NewError(status int, message string, errs ...error) StatusError {
	return &statusError{status: status, message: message}
}

// Convenience constructors matching the real huma API.

func Error400BadRequest(msg string, errs ...error) StatusError {
	return NewError(400, msg, errs...)
}

func Error404NotFound(msg string, errs ...error) StatusError {
	return NewError(404, msg, errs...)
}

func Error500InternalServerError(msg string, errs ...error) StatusError {
	return NewError(500, msg, errs...)
}
