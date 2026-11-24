package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"smartfood/internal/service"
)

type AuthHandler struct {
	svc      *service.AuthService
	products *service.ProductService
}

func NewAuthHandler(svc *service.AuthService, products *service.ProductService) *AuthHandler {
	return &AuthHandler{svc: svc, products: products}
}

type registerRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Phone     string `json:"phone"`
	BirthDate string `json:"birth_date"` // YYYY-MM-DD
	Address   string `json:"address"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type emailRequest struct {
	Email string `json:"email"`
}

type verifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type resetRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

// POST /api/register -> akun dibuat, OTP dikirim ke email (belum ada token)
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	profile, err := service.ValidateProfile(req.Phone, req.BirthDate, req.Address)
	if err != nil {
		fail(c, err)
		return
	}
	user, err := h.svc.Register(req.Username, req.Email, req.Password, profile)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"message":               "Kode verifikasi dikirim ke email",
		"email":                 user.Email,
		"verification_required": true,
	})
}

// POST /api/verify-email -> email terverifikasi dan langsung dapat token
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	user, token, err := h.svc.VerifyEmail(req.Email, req.Code)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// POST /api/resend-otp -> selalu berhasil (tidak membocorkan email terdaftar)
func (h *AuthHandler) ResendOTP(c *gin.Context) {
	var req emailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	if err := h.svc.ResendOTP(req.Email); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Jika email terdaftar, kode baru dikirim"})
}

// POST /api/forgot-password -> selalu berhasil (tidak membocorkan email terdaftar)
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req emailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	if err := h.svc.ForgotPassword(req.Email); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Jika email terdaftar, kode reset dikirim"})
}

// POST /api/reset-password
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req resetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	if err := h.svc.ResetPassword(req.Email, req.Code, req.NewPassword); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password berhasil diubah, silakan masuk"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	user, token, err := h.svc.Login(req.Email, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// Me dipakai Splash untuk memeriksa sesi login.
func (h *AuthHandler) Me(c *gin.Context) {
	user, err := h.svc.GetUser(c.GetInt("user_id"))
	if errors.Is(err, service.ErrNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "pengguna tidak ditemukan"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	// product = produk milik user (null kalau belum punya); dipakai app untuk menentukan halaman
	mp, err := h.products.MyProduct(user.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "product": mp})
}

type profileRequest struct {
	Username string `json:"username"`
}

// PUT /api/me  {"username": "..."}
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req profileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	user, err := h.svc.UpdateUsername(c.GetInt("user_id"), req.Username)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

type detailsRequest struct {
	Phone     string `json:"phone"`
	BirthDate string `json:"birth_date"`
	Address   string `json:"address"`
}

// PUT /api/me/details  {"phone": "...", "birth_date": "YYYY-MM-DD", "address": "..."}
func (h *AuthHandler) UpdateDetails(c *gin.Context) {
	var req detailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	user, err := h.svc.UpdateDetails(c.GetInt("user_id"), req.Phone, req.BirthDate, req.Address)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

type fullProfileRequest struct {
	Username  string `json:"username"`
	Phone     string `json:"phone"`
	BirthDate string `json:"birth_date"`
	Address   string `json:"address"`
}

// PUT /api/me/profile  {"username": "...", "phone": "...", "birth_date": "YYYY-MM-DD", "address": "..."}
// Username dan data diri disimpan sekaligus (dipakai tab Edit di halaman Profil).
// Tanggal lahir hanya bisa diubah seminggu sekali (400 kalau masih masa tunggu).
func (h *AuthHandler) UpdateFullProfile(c *gin.Context) {
	var req fullProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	user, err := h.svc.UpdateProfile(c.GetInt("user_id"), req.Username, req.Phone, req.BirthDate, req.Address)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// PUT /api/password  {"current_password": "...", "new_password": "..."}
// Mengembalikan token baru karena token lama tidak berlaku lagi.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req passwordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	token, err := h.svc.ChangePassword(c.GetInt("user_id"), req.CurrentPassword, req.NewPassword)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "message": "Password berhasil diubah"})
}
