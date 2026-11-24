package handler

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"smartfood/internal/service"
)

// fail mengubah error service menjadi respons HTTP yang sesuai.
func fail(c *gin.Context, err error) {
	var ve *service.ValidationError
	switch {
	case errors.As(err, &ve):
		c.JSON(http.StatusBadRequest, gin.H{"error": ve.Message})
	case errors.Is(err, service.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": service.ErrInvalidCredentials.Error()})
	case errors.Is(err, service.ErrEmailNotVerified):
		c.JSON(http.StatusForbidden, gin.H{"error": service.ErrEmailNotVerified.Error(), "code": "email_not_verified"})
	case errors.Is(err, service.ErrOTPInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrOTPInvalid.Error(), "code": "otp_invalid"})
	case errors.Is(err, service.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": service.ErrEmailTaken.Error()})
	case errors.Is(err, service.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": service.ErrUsernameTaken.Error()})
	case errors.Is(err, service.ErrWrongPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrWrongPassword.Error()})
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": service.ErrNotFound.Error()})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": service.ErrForbidden.Error()})
	case errors.Is(err, service.ErrProductLocked):
		c.JSON(http.StatusForbidden, gin.H{"error": service.ErrProductLocked.Error(), "code": "product_locked"})
	case errors.Is(err, service.ErrCodeInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrCodeInvalid.Error(), "code": "code_invalid"})
	case errors.Is(err, service.ErrHasProduct):
		c.JSON(http.StatusConflict, gin.H{"error": service.ErrHasProduct.Error(), "code": "has_product"})
	case errors.Is(err, service.ErrTooMany):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": service.ErrTooMany.Error()})
	case errors.Is(err, service.ErrUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": service.ErrUnavailable.Error()})
	default:
		log.Printf("[error] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "terjadi kesalahan pada server"})
	}
}

func badJSON(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "format JSON tidak valid"})
}

// parseTime menerima RFC3339, "2006-01-02 15:04:05", atau "2006-01-02".
// Untuk tanggal saja pada batas akhir (to), hasilnya akhir hari tersebut.
func parseTime(s string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, service.NewValidationError("format waktu tidak valid: " + s)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Millisecond)
	}
	return t, nil
}
