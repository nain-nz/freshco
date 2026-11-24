package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"smartfood/internal/service"
)

type ProductHandler struct {
	products *service.ProductService
	payments *service.PaymentService
}

func NewProductHandler(p *service.ProductService, pay *service.PaymentService) *ProductHandler {
	return &ProductHandler{products: p, payments: pay}
}

type codeRequest struct {
	Code string `json:"code"`
}

// POST /api/products/claim  {"code": "ABCD-EFGH-JKLM"}  -> user menjadi owner
func (h *ProductHandler) Claim(c *gin.Context) {
	var req codeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	mp, err := h.products.Claim(c.GetInt("user_id"), req.Code)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": mp})
}

// POST /api/products/join  {"code": "ABCD-EFGH"}  -> user menjadi member
func (h *ProductHandler) Join(c *gin.Context) {
	var req codeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	mp, err := h.products.Join(c.GetInt("user_id"), req.Code)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": mp})
}

// GET /api/products/:pid/members
func (h *ProductHandler) Members(c *gin.Context) {
	data, err := h.products.Members(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// DELETE /api/products/:pid/members/:uid  (owner)
func (h *ProductHandler) RemoveMember(c *gin.Context) {
	uid, err := strconv.Atoi(c.Param("uid"))
	if err != nil {
		fail(c, service.NewValidationError("id user tidak valid"))
		return
	}
	if err := h.products.RemoveMember(c.GetInt("product_id"), uid); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "anggota dikeluarkan"})
}

// POST /api/products/:pid/leave  (member keluar dari kelompok)
func (h *ProductHandler) Leave(c *gin.Context) {
	if c.GetString("member_role") == "owner" {
		fail(c, service.NewValidationError("owner tidak bisa keluar dari produknya"))
		return
	}
	if err := h.products.RemoveMember(c.GetInt("product_id"), c.GetInt("user_id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "kamu keluar dari kelompok"})
}

// POST /api/products/:pid/invite  (owner) -> kode baru, kode lama mati
func (h *ProductHandler) CreateInvite(c *gin.Context) {
	code, exp, err := h.products.CreateInvite(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": code, "expires_at": exp})
}

// GET /api/products/:pid/invite  (owner) -> apakah ada kode aktif
func (h *ProductHandler) ActiveInvite(c *gin.Context) {
	exp, err := h.products.ActiveInviteExpiry(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"active": exp != nil, "expires_at": exp})
}

// tagihan (owner)

// GET /api/products/:pid/billing
func (h *ProductHandler) Billing(c *gin.Context) {
	v, err := h.payments.Billing(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

type orderRequest struct {
	Tenor int `json:"tenor"`
}

// POST /api/products/:pid/order  {"tenor": 1|2|4|6}
func (h *ProductHandler) CreateOrder(c *gin.Context) {
	var req orderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	v, err := h.payments.CreateOrder(c.GetInt("user_id"), c.GetInt("product_id"), req.Tenor)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func payID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("payid"))
	if err != nil {
		fail(c, service.NewValidationError("id tagihan tidak valid"))
		return 0, false
	}
	return id, true
}

// POST /api/products/:pid/payments/:payid/pay  -> QRIS (qr_string, expires_at, seconds_left); QR yang masih berlaku dipakai ulang
func (h *ProductHandler) Pay(c *gin.Context) {
	id, ok := payID(c)
	if !ok {
		return
	}
	res, err := h.payments.Pay(c.GetInt("user_id"), c.GetInt("product_id"), id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// POST /api/products/:pid/payments/:payid/check  -> tanya status ke penyedia QRIS
func (h *ProductHandler) Check(c *gin.Context) {
	id, ok := payID(c)
	if !ok {
		return
	}
	st, err := h.payments.Check(c.GetInt("product_id"), id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": st})
}
