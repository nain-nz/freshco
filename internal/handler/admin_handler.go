package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"smartfood/internal/model"
	"smartfood/internal/payment"
	"smartfood/internal/service"
)

type AdminHandler struct {
	products *service.ProductService
	payments *service.PaymentService
	users    *service.AuthService
}

func NewAdminHandler(p *service.ProductService, pay *service.PaymentService, u *service.AuthService) *AdminHandler {
	return &AdminHandler{products: p, payments: pay, users: u}
}

func idParam(c *gin.Context, name string) (int, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil {
		fail(c, service.NewValidationError("id tidak valid"))
		return 0, false
	}
	return id, true
}

// GET /api/admin/products?q=
func (h *AdminHandler) List(c *gin.Context) {
	data, err := h.products.AdminList(c.Query("q"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// POST /api/admin/products  -> serial + kode klaim (kode hanya tampil sekali)
func (h *AdminHandler) Create(c *gin.Context) {
	p, code, err := h.products.Create()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"product": p, "claim_code": code})
}

// GET /api/admin/products/:id
func (h *AdminHandler) Get(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	p, err := h.products.AdminGet(id)
	if err != nil {
		fail(c, err)
		return
	}
	o, ps, err := h.payments.OrderWithPayments(id)
	if err != nil {
		fail(c, err)
		return
	}
	members, err := h.products.Members(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, model.AdminProductDetail{AdminProduct: *p, Members: members, Order: o, Payments: ps})
}

// DELETE /api/admin/products/:id  (hanya produk yang belum diklaim)
func (h *AdminHandler) Delete(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	if err := h.products.Delete(id); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "produk dihapus"})
}

// GET /api/admin/users?q=
func (h *AdminHandler) Users(c *gin.Context) {
	data, err := h.users.AdminUsers(c.Query("q"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GET /api/admin/users/:id  -> data diri + produk user
func (h *AdminHandler) UserDetail(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	d, err := h.users.AdminUserDetail(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

// POST /api/admin/products/:id/reset-claim
func (h *AdminHandler) ResetClaim(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	code, err := h.products.ResetClaimCode(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"claim_code": code})
}

type statusRequest struct {
	Status string `json:"status"`
}

// POST /api/admin/products/:id/status  {"status": "active"|"suspended"}
func (h *AdminHandler) SetStatus(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badJSON(c)
		return
	}
	if err := h.products.SetStatus(id, req.Status); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "status diperbarui"})
}

// POST /api/admin/payments/:id/confirm-cash
func (h *AdminHandler) ConfirmCash(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	if err := h.payments.ConfirmCash(c.GetInt("user_id"), id); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "pembayaran cash dikonfirmasi"})
}

// POST /api/payments/notify  (publik; webhook Midtrans, dijaga tanda tangan)
func (h *AdminHandler) Notify(c *gin.Context) {
	var n payment.Notification
	if err := c.ShouldBindJSON(&n); err != nil {
		badJSON(c)
		return
	}
	if err := h.payments.HandleNotification(n); err != nil {
		if err == service.ErrForbidden {
			log.Printf("[payment] notifikasi dengan tanda tangan salah: %s", n.OrderID)
			c.JSON(http.StatusForbidden, gin.H{"error": "signature tidak valid"})
			return
		}
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
