package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"smartfood/internal/model"
	"smartfood/internal/service"
)

// AuthRequired mewajibkan header "Authorization: Bearer <token>".
// authenticate memvalidasi token dan memuat user dari database (AuthService.Authenticate),
// jadi token lama (setelah ganti password) ditolak.
func AuthRequired(authenticate func(string) (*model.User, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token tidak ada"})
			return
		}
		u, err := authenticate(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid atau kedaluwarsa"})
			return
		}
		c.Set("user_id", u.ID)
		c.Set("role", u.Role)
		c.Next()
	}
}

// AdminRequired: hanya users.role = 'admin'.
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "khusus admin"})
			return
		}
		c.Next()
	}
}

// ProductMember memastikan user adalah anggota produk :pid. Menyimpan
// product_id, member_role (owner|member), dan product_status di context.
func ProductMember(membership func(userID, pid int) (string, string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid, err := strconv.Atoi(c.Param("pid"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "id produk tidak valid"})
			return
		}
		role, status, err := membership(c.GetInt("user_id"), pid)
		if errors.Is(err, service.ErrForbidden) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": service.ErrForbidden.Error()})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "terjadi kesalahan pada server"})
			return
		}
		c.Set("product_id", pid)
		c.Set("member_role", role)
		c.Set("product_status", status)
		c.Next()
	}
}

// OwnerOnly: hanya owner produk.
func OwnerOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("member_role") != model.RoleOwner {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "khusus owner produk"})
			return
		}
		c.Next()
	}
}

// ProductActive: data dan kontrol produk hanya terbuka kalau produk berstatus active.
// pending_payment dan suspended hanya boleh membuka halaman tagihan.
func ProductActive() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("product_status") != model.StatusActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": service.ErrProductLocked.Error(), "code": "product_locked",
				"product_status": c.GetString("product_status"),
			})
			return
		}
		c.Next()
	}
}
