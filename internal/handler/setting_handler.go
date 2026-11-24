package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"smartfood/internal/service"
)

type SettingHandler struct{ svc *service.SettingService }

func NewSettingHandler(svc *service.SettingService) *SettingHandler {
	return &SettingHandler{svc: svc}
}

// GET /api/products/:pid/settings
func (h *SettingHandler) Get(c *gin.Context) {
	st, err := h.svc.Get(c.GetInt("product_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

// PUT /api/products/:pid/settings  (semua field opsional)
func (h *SettingHandler) Update(c *gin.Context) {
	var in service.UpdateSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badJSON(c)
		return
	}
	res, err := h.svc.Update(c.GetInt("product_id"), in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
