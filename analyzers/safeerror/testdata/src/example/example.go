package example

import (
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// --- Should be flagged ---

func badNewError(err error) error {
	return huma.NewError(http.StatusBadRequest, err.Error()) // want "raw err.Error\\(\\) passed to NewError"
}

func badNewError5xx(err error) error {
	return huma.NewError(http.StatusInternalServerError, err.Error()) // want "raw err.Error\\(\\) passed to NewError"
}

func bad400(err error) error {
	return huma.Error400BadRequest(err.Error()) // want "raw err.Error\\(\\) passed to Error400BadRequest"
}

func bad404(err error) error {
	return huma.Error404NotFound(err.Error()) // want "raw err.Error\\(\\) passed to Error404NotFound"
}

func bad500(err error) error {
	return huma.Error500InternalServerError(err.Error()) // want "raw err.Error\\(\\) passed to Error500InternalServerError"
}

func badWrappedError(err error) error {
	return huma.Error400BadRequest(fmt.Errorf("wrap: %w", err).Error()) // want "raw err.Error\\(\\) passed to Error400BadRequest"
}

func badNolintStillLeaks(err error) error {
	//nolint:safeerror -- retired bypass must not hide a client-facing leak
	return huma.Error400BadRequest(err.Error()) // want "raw err.Error\\(\\) passed to Error400BadRequest"
}

// --- Should NOT be flagged ---

func okStringLiteral() error {
	return huma.NewError(http.StatusBadRequest, "invalid input")
}

func okFmtSprintf(id string) error {
	return huma.NewError(http.StatusNotFound, fmt.Sprintf("user %s not found", id))
}

func okConvenienceStringLiteral() error {
	return huma.Error400BadRequest("email is required")
}

func okErrInVariable(err error) error {
	msg := err.Error()
	// Indirect — out of scope for AST-level analysis.
	return huma.NewError(http.StatusBadRequest, msg)
}
