//go:build integration

package main

// Tes alur lengkap lewat HTTP: admin membuat produk, klaim, pilih bayar, konfirmasi cash,
// undangan, pemisahan data antar produk, suspend karena telat, webhook Midtrans.
// Butuh database kosong yang sudah di-migrate + seed (DB_* dari environment):
//   go test -tags integration -run TestFlow -v .

import (
	"bytes"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"smartfood/internal/config"
	"smartfood/internal/database"
	"smartfood/internal/mailer"
	"smartfood/internal/model"
	"smartfood/internal/payment"
	"smartfood/internal/service"
)

type fakePub struct{ serials []string }

func (f *fakePub) PublishSettings(serial string, _ model.Settings) error {
	f.serials = append(f.serials, serial)
	return nil
}

type env struct {
	t    *testing.T
	h    http.Handler
	db   *sql.DB
	auth *service.AuthService
	pay  *service.PaymentService
	cfg  config.Config
}

func (e *env) call(method, path, token string, body any) (int, map[string]any) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *env) want(status int, got int, body map[string]any, what string) {
	e.t.Helper()
	if got != status {
		e.t.Fatalf("%s: status %d, mau %d, body=%v", what, got, status, body)
	}
}

func (e *env) newUser(name string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.MinCost)
	_, err := e.db.Exec("INSERT INTO users (username, email, password, email_verified_at) VALUES (?, ?, ?, NOW())",
		name, name+"@example.com", string(hash))
	if err != nil {
		e.t.Fatal(err)
	}
	_, tok, err := e.auth.Login(name+"@example.com", "Passw0rd!")
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

func f(v any, k string) any { return v.(map[string]any)[k] }

func TestFlow(t *testing.T) {
	cfg := config.Load()
	cfg.MidtransServerKey = "test-server-key"
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mail, _ := mailer.New(cfg)
	secret := []byte(cfg.JWTSecret)
	otp := service.NewOTPService(db, mail, secret, "Test", "")
	auth := service.NewAuthService(db, otp, secret)
	products := service.NewProductService(db, cfg)
	pay := service.NewPaymentService(db, cfg, payment.NewMidtrans(cfg.MidtransServerKey, true))
	pub := &fakePub{}
	h := newRouter(deps{
		Auth: auth, Product: products, Payment: pay,
		Reading: service.NewReadingService(db), Alert: service.NewAlertService(db),
		Setting: service.NewSettingService(db, pub),
	})
	e := &env{t: t, h: h, db: db, auth: auth, pay: pay, cfg: cfg}

	suffix := fmt.Sprint(time.Now().UnixNano() % 1000000)
	admin := e.newUser("adm" + suffix)
	if _, err := db.Exec("UPDATE users SET role='admin' WHERE username=?", "adm"+suffix); err != nil {
		t.Fatal(err)
	}
	_, admin, _ = auth.Login("adm"+suffix+"@example.com", "Passw0rd!") // token baru setelah role berubah
	a, b, c, d := e.newUser("a"+suffix), e.newUser("b"+suffix), e.newUser("c"+suffix), e.newUser("d"+suffix)

	// ---- admin membuat produk ----
	st, body := e.call("POST", "/api/admin/products", a, nil)
	e.want(403, st, body, "user biasa tidak boleh membuat produk")
	st, body = e.call("POST", "/api/admin/products", admin, nil)
	e.want(201, st, body, "admin membuat produk")
	claim := body["claim_code"].(string)
	pid := int(f(body["product"], "id").(float64))
	serial := f(body["product"], "serial").(string)
	if len(claim) != 14 || claim[4] != '-' || !strings.HasPrefix(serial, "FC-") {
		t.Fatalf("format kode salah: %q %q", claim, serial)
	}
	var nSet int
	db.QueryRow("SELECT COUNT(*) FROM settings WHERE product_id=?", pid).Scan(&nSet)
	if nSet != 11 {
		t.Fatalf("pengaturan bawaan %d baris, mau 11", nSet)
	}
	t.Logf("produk %d serial=%s", pid, serial)

	// ---- klaim ----
	st, body = e.call("GET", "/api/me", a, nil)
	e.want(200, st, body, "me")
	if body["product"] != nil {
		t.Fatal("user baru seharusnya belum punya produk")
	}
	st, body = e.call("POST", "/api/products/claim", a, map[string]string{"code": "AAAA-BBBB-CCCC"})
	e.want(400, st, body, "kode klaim salah")
	if body["code"] != "code_invalid" {
		t.Fatalf("code=%v", body["code"])
	}
	st, body = e.call("POST", "/api/products/claim", a, map[string]string{"code": strings.ToLower(strings.ReplaceAll(claim, "-", " "))})
	e.want(200, st, body, "klaim (huruf kecil, spasi) harus tetap benar")
	if f(body["product"], "role") != "owner" || f(body["product"], "status") != "pending_payment" {
		t.Fatalf("hasil klaim: %v", body)
	}
	st, body = e.call("POST", "/api/products/claim", c, map[string]string{"code": claim})
	e.want(400, st, body, "kode klaim hanya sekali pakai")

	P := fmt.Sprintf("/api/products/%d", pid)

	// ---- terkunci sebelum DP lunas ----
	st, body = e.call("GET", P+"/latest", a, nil)
	e.want(403, st, body, "data terkunci sebelum DP lunas")
	if body["code"] != "product_locked" {
		t.Fatalf("code=%v", body["code"])
	}
	st, body = e.call("GET", P+"/settings", a, nil)
	e.want(403, st, body, "settings terkunci")

	// ---- pilih pembayaran ----
	st, body = e.call("GET", P+"/billing", a, nil)
	e.want(200, st, body, "billing")
	if body["order"] != nil || int(body["price"].(float64)) != 500000 || int(body["dp"].(float64)) != 100000 {
		t.Fatalf("billing awal: %v", body)
	}
	st, body = e.call("POST", P+"/order", a, map[string]int{"tenor": 3})
	e.want(400, st, body, "tenor 3 tidak valid")
	st, body = e.call("POST", P+"/order", a, map[string]int{"tenor": 2})
	e.want(201, st, body, "order tenor 2")
	pays := body["payments"].([]any)
	var ids []int
	for i, w := range []int{100000, 200000, 200000} {
		if int(f(pays[i], "amount").(float64)) != w || int(f(pays[i], "seq").(float64)) != i {
			t.Fatalf("tagihan %d: %v", i, pays[i])
		}
		ids = append(ids, int(f(pays[i], "id").(float64)))
	}
	st, body = e.call("POST", P+"/order", a, map[string]int{"tenor": 4})
	e.want(400, st, body, "pilihan hanya sekali")

	// ---- cash: admin konfirmasi DP, produk aktif ----
	st, body = e.call("POST", fmt.Sprintf("/api/admin/payments/%d/confirm-cash", ids[0]), a, nil)
	e.want(403, st, body, "owner tidak boleh konfirmasi cash")
	st, body = e.call("POST", fmt.Sprintf("/api/admin/payments/%d/confirm-cash", ids[0]), admin, nil)
	e.want(200, st, body, "admin konfirmasi DP")
	st, body = e.call("POST", fmt.Sprintf("/api/admin/payments/%d/confirm-cash", ids[0]), admin, nil)
	e.want(200, st, body, "konfirmasi ulang idempoten")
	st, body = e.call("GET", P+"/latest", a, nil)
	e.want(200, st, body, "data terbuka setelah DP lunas")
	st, body = e.call("GET", "/api/me", a, nil)
	if f(body["product"], "status") != "active" || f(body["product"], "has_order") != true {
		t.Fatalf("me setelah DP: %v", body["product"])
	}
	var method string
	var confirmedBy sql.NullInt64
	db.QueryRow("SELECT method, confirmed_by FROM payments WHERE id=?", ids[0]).Scan(&method, &confirmedBy)
	if method != "cash" || !confirmedBy.Valid {
		t.Fatalf("method=%q confirmed_by=%v", method, confirmedBy)
	}

	// ---- kelompok ----
	st, body = e.call("POST", "/api/products/join", b, map[string]string{"code": "ZZZZ-ZZZZ"})
	e.want(400, st, body, "undangan salah")
	st, body = e.call("POST", P+"/invite", b, nil)
	e.want(403, st, body, "member/non-anggota tidak boleh membuat undangan")
	st, body = e.call("POST", P+"/invite", a, nil)
	e.want(201, st, body, "owner membuat undangan")
	old := body["code"].(string)
	st, body = e.call("POST", P+"/invite", a, nil)
	e.want(201, st, body, "undangan baru")
	newCode := body["code"].(string)
	st, body = e.call("POST", "/api/products/join", c, map[string]string{"code": old})
	e.want(400, st, body, "kode lama mati setelah dibuat kode baru")
	st, body = e.call("POST", "/api/products/join", b, map[string]string{"code": newCode})
	e.want(200, st, body, "b bergabung")
	if f(body["product"], "role") != "member" {
		t.Fatalf("role=%v", f(body["product"], "role"))
	}
	st, body = e.call("POST", "/api/products/join", c, map[string]string{"code": newCode})
	e.want(400, st, body, "kode undangan hanya untuk 1 user")
	st, body = e.call("GET", P+"/invite", a, nil)
	if body["active"] != false {
		t.Fatalf("undangan terpakai tidak boleh aktif: %v", body)
	}

	// member: boleh data & kontrol, tidak boleh tagihan / kelola anggota
	st, body = e.call("PUT", P+"/settings", b, map[string]any{"target_temp": 6})
	e.want(200, st, body, "member boleh mengubah pengaturan")
	if len(pub.serials) == 0 || pub.serials[len(pub.serials)-1] != serial {
		t.Fatalf("pengaturan harus dikirim ke serial %s, dapat %v", serial, pub.serials)
	}
	st, body = e.call("GET", P+"/billing", b, nil)
	e.want(403, st, body, "member tidak boleh melihat tagihan")
	st, body = e.call("POST", P+"/payments/1/pay", b, nil)
	e.want(403, st, body, "member tidak boleh membayar")
	st, body = e.call("GET", P+"/members", b, nil)
	e.want(200, st, body, "member melihat anggota")
	if len(body["data"].([]any)) != 2 {
		t.Fatalf("anggota: %v", body)
	}
	st, body = e.call("GET", P+"/members", c, nil)
	e.want(403, st, body, "non-anggota tidak boleh melihat anggota")

	// ---- satu user satu produk ----
	st, body = e.call("POST", "/api/admin/products", admin, nil)
	e.want(201, st, body, "produk ke-2")
	claim2 := body["claim_code"].(string)
	pid2 := int(f(body["product"], "id").(float64))
	st, body = e.call("POST", "/api/products/claim", a, map[string]string{"code": claim2})
	e.want(409, st, body, "a sudah punya produk")
	st, body = e.call("POST", "/api/products/claim", d, map[string]string{"code": claim2})
	e.want(200, st, body, "d klaim produk 2")

	// ---- pemisahan data antar produk ----
	var chAtas int
	db.QueryRow("SELECT id FROM channels WHERE name='suhu_atas'").Scan(&chAtas)
	db.Exec("INSERT INTO readings (product_id, `time`, channel_id, `value`) VALUES (?, NOW(3), ?, 11.5), (?, NOW(3), ?, 22.5)", pid, chAtas, pid2, chAtas)
	st, body = e.call("GET", P+"/latest", a, nil)
	e.want(200, st, body, "latest a")
	vals := body["data"].([]any)
	if len(vals) != 1 || f(vals[0], "value").(float64) != 11.5 {
		t.Fatalf("a hanya boleh melihat data produknya: %v", vals)
	}
	st, body = e.call("GET", fmt.Sprintf("/api/products/%d/latest", pid2), a, nil)
	e.want(403, st, body, "a tidak boleh membaca produk lain")
	st, body = e.call("GET", fmt.Sprintf("/api/products/%d/latest", pid), d, nil)
	e.want(403, st, body, "d tidak boleh membaca produk a")
	db.Exec("INSERT INTO alerts (product_id, `time`, channel_id, type, `value`) VALUES (?, NOW(), NULL, 'suhu_tinggi', 15)", pid2)
	st, body = e.call("GET", P+"/alerts", a, nil)
	e.want(200, st, body, "alerts a")
	if len(body["data"].([]any)) != 0 {
		t.Fatalf("alert produk lain bocor: %v", body)
	}

	// ---- admin: daftar & detail ----
	st, body = e.call("GET", "/api/admin/products?q=a"+suffix, admin, nil)
	e.want(200, st, body, "admin cari")
	if l := body["data"].([]any); len(l) != 1 || f(l[0], "member_count").(float64) != 2 {
		t.Fatalf("pencarian admin: %v", body)
	}
	st, body = e.call("GET", fmt.Sprintf("/api/admin/products/%d", pid), admin, nil)
	e.want(200, st, body, "admin detail")
	if len(body["payments"].([]any)) != 3 || body["owner_username"] != "a"+suffix {
		t.Fatalf("detail: %v", body)
	}
	st, body = e.call("POST", fmt.Sprintf("/api/admin/products/%d/reset-claim", pid), admin, nil)
	e.want(400, st, body, "reset kode klaim hanya produk belum diklaim")

	// ---- cicilan telat -> suspend -> lunas -> aktif lagi ----
	db.Exec("UPDATE payments SET due_date = DATE_SUB(CURDATE(), INTERVAL 30 DAY) WHERE id IN (?, ?)", ids[1], ids[2])
	n, err := e.pay.SuspendOverdue()
	if err != nil || n != 1 {
		t.Fatalf("SuspendOverdue n=%d err=%v", n, err)
	}
	st, body = e.call("GET", P+"/latest", a, nil)
	e.want(403, st, body, "suspended: data terkunci")
	st, body = e.call("GET", P+"/billing", a, nil)
	e.want(200, st, body, "suspended: tagihan tetap bisa dibuka owner")
	e.call("POST", fmt.Sprintf("/api/admin/payments/%d/confirm-cash", ids[1]), admin, nil)
	st, _ = e.call("GET", P+"/latest", a, nil)
	if st != 403 {
		t.Fatalf("masih ada cicilan telat, harus tetap suspended (status %d)", st)
	}
	e.call("POST", fmt.Sprintf("/api/admin/payments/%d/confirm-cash", ids[2]), admin, nil)
	st, body = e.call("GET", P+"/latest", a, nil)
	e.want(200, st, body, "semua telat lunas -> aktif lagi")

	// ---- webhook Midtrans ----
	pid3 := ids[0] // tagihan yang sudah lunas: dipakai untuk uji idempoten
	_ = pid3
	db.Exec("INSERT INTO orders (product_id, user_id, total, dp, tenor, created_at) VALUES (?, (SELECT id FROM users WHERE username=?), 500000, 100000, 1, NOW())", pid2, "d"+suffix)
	var oid2 int
	db.QueryRow("SELECT id FROM orders WHERE product_id=?", pid2).Scan(&oid2)
	res, _ := db.Exec("INSERT INTO payments (order_id, seq, amount, due_date, method, provider_order_id, status, created_at) VALUES (?, 0, 100000, CURDATE(), 'qris', ?, 'pending', NOW())", oid2, "SFBTEST-"+suffix)
	payID, _ := res.LastInsertId()
	sig := func(order, code, gross string) string {
		s := sha512.Sum512([]byte(order + code + gross + cfg.MidtransServerKey))
		return hex.EncodeToString(s[:])
	}
	notif := map[string]string{"order_id": "SFBTEST-" + suffix, "status_code": "200", "gross_amount": "100000.00",
		"transaction_status": "settlement", "fraud_status": "accept", "signature_key": "salah"}
	st, body = e.call("POST", "/api/payments/notify", "", notif)
	e.want(403, st, body, "webhook tanda tangan salah")
	notif["signature_key"] = sig("SFBTEST-"+suffix, "200", "100000.00")
	st, body = e.call("POST", "/api/payments/notify", "", notif)
	e.want(200, st, body, "webhook valid")
	st, body = e.call("POST", "/api/payments/notify", "", notif)
	e.want(200, st, body, "webhook berulang (idempoten)")
	var pst string
	db.QueryRow("SELECT status FROM payments WHERE id=?", payID).Scan(&pst)
	var prodSt string
	db.QueryRow("SELECT status FROM products WHERE id=?", pid2).Scan(&prodSt)
	if pst != "paid" || prodSt != "active" {
		t.Fatalf("setelah webhook: payment=%s product=%s", pst, prodSt)
	}
	// notifikasi order_id tak dikenal diabaikan (200)
	n2 := map[string]string{"order_id": "X-1", "status_code": "200", "gross_amount": "1.00", "transaction_status": "settlement"}
	n2["signature_key"] = sig("X-1", "200", "1.00")
	st, body = e.call("POST", "/api/payments/notify", "", n2)
	e.want(200, st, body, "order_id tak dikenal")

	// ---- keluar / dikeluarkan ----
	st, body = e.call("POST", P+"/leave", a, nil)
	e.want(400, st, body, "owner tidak bisa keluar")
	st, body = e.call("DELETE", fmt.Sprintf("%s/members/%d", P, 0), a, nil)
	e.want(404, st, body, "hapus anggota yang tidak ada")
	var bID int
	db.QueryRow("SELECT id FROM users WHERE username=?", "b"+suffix).Scan(&bID)
	st, body = e.call("DELETE", fmt.Sprintf("%s/members/%d", P, bID), a, nil)
	e.want(200, st, body, "owner mengeluarkan b")
	st, body = e.call("GET", P+"/latest", b, nil)
	e.want(403, st, body, "b tidak punya akses lagi")

	// ---- batas percobaan kode ----
	var got429 bool
	for i := 0; i < 12; i++ {
		st, _ = e.call("POST", "/api/products/join", c, map[string]string{"code": "AAAA-AAAA"})
		if st == 429 {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("percobaan kode berulang seharusnya kena batas (429)")
	}
}
