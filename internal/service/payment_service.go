package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"smartfood/internal/config"
	"smartfood/internal/model"
	"smartfood/internal/payment"
)

type PaymentService struct {
	db  *sql.DB
	cfg config.Config
	mt  *payment.Midtrans

	// Saat webhook aktif, Midtrans tetap ditanya sesekali sebagai cadangan kalau webhook
	// tidak sampai (mis. alamat tunnel berubah). lastAsk mencatat kapan terakhir ditanya.
	askMu   sync.Mutex
	lastAsk map[int]time.Time
}

// webhookFallbackEvery: jarak minimal tanya-ke-Midtrans per tagihan saat webhook aktif.
const webhookFallbackEvery = 30 * time.Second

func NewPaymentService(db *sql.DB, cfg config.Config, mt *payment.Midtrans) *PaymentService {
	return &PaymentService{db: db, cfg: cfg, mt: mt, lastAsk: map[int]time.Time{}}
}

// shouldAsk true kalau tagihan ini boleh ditanyakan ke Midtrans sekarang.
func (s *PaymentService) shouldAsk(payID int) bool {
	if !s.mt.WebhookEnabled() {
		return true
	}
	s.askMu.Lock()
	defer s.askMu.Unlock()
	if t, ok := s.lastAsk[payID]; ok && time.Since(t) < webhookFallbackEvery {
		return false
	}
	s.lastAsk[payID] = time.Now()
	return true
}

// Installment = satu baris jadwal tagihan.
type Installment struct {
	Seq    int
	Amount int // total ditagihkan = bagian produk + Fee
	Fee    int // biaya langganan di dalam Amount (0 untuk DP)
	Due    time.Time
}

// BuildSchedule membuat jadwal tagihan: DP, lalu sisa harga dibagi tenor.
// fee = langganan per bulan. Kalau dicicil (tenor > 1), fee digabung ke tiap cicilan
// supaya user cukup bayar sekali sebulan (Amount sudah termasuk fee, jadi nominal
// QRIS/cash langsung benar). Bayar langsung (tenor 1) jatuh tempo hari ini, jadi tanpa
// fee; langganannya baru ditagih bulan depan lewat ensureSubscription.
func BuildSchedule(total, dp, tenor, fee int, today time.Time) []Installment {
	out := []Installment{{Seq: 0, Amount: dp, Due: today}}
	rest := total - dp
	per := rest / tenor
	for k := 1; k <= tenor; k++ {
		amt := per
		if k == tenor {
			amt = rest - per*(tenor-1)
		}
		due := today
		f := 0
		if tenor > 1 {
			due = today.AddDate(0, k, 0)
			f = fee
		}
		out = append(out, Installment{Seq: k, Amount: amt + f, Fee: f, Due: due})
	}
	return out
}

func validTenor(t int) bool { return t == 1 || t == 2 || t == 4 || t == 6 }

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.Local)
}

// order dan tagihan

type BillingView struct {
	Order         *model.Order    `json:"order"`
	Payments      []model.Payment `json:"payments"`
	ProductStatus string          `json:"product_status"`
	Price         int             `json:"price"`
	DP            int             `json:"dp"`
	// SubscriptionFee: biaya langganan per bulan (sudah termasuk di tiap tagihan cicilan baru).
	SubscriptionFee int `json:"subscription_fee"`
}

// CreateOrder: owner memilih bayar langsung / cicilan; jadwal tagihan dibuat.
func (s *PaymentService) CreateOrder(userID, pid, tenor int) (*BillingView, error) {
	if !validTenor(tenor) {
		return nil, NewValidationError("tenor harus 1 (langsung), 2, 4, atau 6")
	}
	total, dp := s.cfg.ProductPrice, s.cfg.DPAmount
	if dp <= 0 || dp >= total {
		return nil, errors.New("konfigurasi harga/DP tidak valid")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	res, err := tx.Exec(
		"INSERT INTO orders (product_id, user_id, total, dp, tenor, subscription_fee, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		pid, userID, total, dp, tenor, s.cfg.SubscriptionFee, now)
	if isDuplicate(err) {
		return nil, NewValidationError("cara pembayaran sudah dipilih")
	}
	if err != nil {
		return nil, err
	}
	oid, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	for _, in := range BuildSchedule(total, dp, tenor, s.cfg.SubscriptionFee, today()) {
		if _, err := tx.Exec(
			"INSERT INTO payments (order_id, seq, amount, fee, due_date, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)",
			oid, in.Seq, in.Amount, in.Fee, in.Due, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Billing(pid)
}

// Billing = order + tagihan sebuah produk (order null kalau belum dipilih).
func (s *PaymentService) Billing(pid int) (*BillingView, error) {
	var oid int
	err := s.db.QueryRow("SELECT id FROM orders WHERE product_id = ?", pid).Scan(&oid)
	if err == nil {
		// tagihan langganan bulan berjalan dibuat saat dibutuhkan (lihat ensureSubscription)
		if err := s.ensureSubscription(oid); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	o, ps, err := s.OrderWithPayments(pid)
	if err != nil {
		return nil, err
	}
	v := &BillingView{Order: o, Payments: ps, Price: s.cfg.ProductPrice, DP: s.cfg.DPAmount, SubscriptionFee: s.cfg.SubscriptionFee}
	if o != nil {
		v.SubscriptionFee = o.SubscriptionFee
	}
	if err := s.db.QueryRow("SELECT status FROM products WHERE id = ?", pid).Scan(&v.ProductStatus); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *PaymentService) OrderWithPayments(pid int) (*model.Order, []model.Payment, error) {
	var o model.Order
	err := s.db.QueryRow(
		"SELECT id, product_id, user_id, total, dp, tenor, subscription_fee, created_at FROM orders WHERE product_id = ?", pid).
		Scan(&o.ID, &o.ProductID, &o.UserID, &o.Total, &o.DP, &o.Tenor, &o.SubscriptionFee, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, []model.Payment{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.Query(
		"SELECT id, order_id, seq, amount, fee, due_date, method, provider_order_id, qr_expires_at, status, paid_at "+
			"FROM payments WHERE order_id = ? ORDER BY seq", o.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	ps := []model.Payment{}
	for rows.Next() {
		var p model.Payment
		var method, pref sql.NullString
		var paid, qrExp sql.NullTime
		if err := rows.Scan(&p.ID, &p.OrderID, &p.Seq, &p.Amount, &p.Fee, &p.DueDate, &method, &pref, &qrExp, &p.Status, &paid); err != nil {
			return nil, nil, err
		}
		if qrExp.Valid {
			t := qrExp.Time
			p.QRExpiresAt = &t
		}
		if method.Valid {
			p.Method = &method.String
		}
		if paid.Valid {
			t := paid.Time
			p.PaidAt = &t
		}
		ps = append(ps, p)
	}
	return &o, ps, rows.Err()
}

// langganan

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// ensureSubscription membuat tagihan langganan yang berdiri sendiri (tanpa cicilan produk),
// yaitu untuk bulan-bulan setelah cicilan habis (atau mulai bulan pertama kalau bayar langsung).
// Bulan yang masih ada cicilannya tidak diurus di sini: fee-nya sudah digabung ke tagihan
// cicilan oleh BuildSchedule. Tiap panggilan membuat tagihan sampai ada satu yang jatuh
// tempo di masa depan, jadi aman dipanggil berulang (idempoten) dan dari mana saja.
// Langganan baru berjalan setelah DP lunas.
func (s *PaymentService) ensureSubscription(orderID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var fee int
	if err := tx.QueryRow("SELECT subscription_fee FROM orders WHERE id = ? FOR UPDATE", orderID).Scan(&fee); err != nil {
		return err
	}
	if fee <= 0 {
		return nil // order lama / tanpa langganan
	}
	var start time.Time
	var dpStatus string
	if err := tx.QueryRow("SELECT due_date, status FROM payments WHERE order_id = ? AND seq = 0", orderID).
		Scan(&start, &dpStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if dpStatus != model.PayPaid {
		return nil
	}
	start = dateOnly(start)

	// bulan yang sudah tertutup langganan = jumlah tagihan yang memuat fee
	var covered, maxSeq int
	if err := tx.QueryRow(
		"SELECT COUNT(CASE WHEN fee > 0 THEN 1 END), COALESCE(MAX(seq), 0) FROM payments WHERE order_id = ?",
		orderID).Scan(&covered, &maxSeq); err != nil {
		return err
	}

	now := time.Now()
	day := today()
	created := false
	for !start.AddDate(0, covered, 0).After(day) {
		covered++
		maxSeq++
		if _, err := tx.Exec(
			"INSERT INTO payments (order_id, seq, amount, fee, due_date, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)",
			orderID, maxSeq, fee, fee, start.AddDate(0, covered, 0), now); err != nil {
			return err
		}
		created = true
	}
	if !created {
		return nil
	}
	return tx.Commit()
}

// bayar lewat QRIS

// PayResult = QR yang ditampilkan aplikasi. SecondsLeft dihitung di server supaya
// hitung mundur di layar sama dengan yang berlaku walau jam HP tidak tepat.
type PayResult struct {
	OrderID  string `json:"provider_order_id"`
	QRString string `json:"qr_string"`
	Amount   int    `json:"amount"`
	// Link gambar QR dari penyedia (ini yang disalin tombol "Salin kode QR").
	QRURL string `json:"qr_url,omitempty"`
	// Jarak antar pengecekan status oleh aplikasi: rapat kalau webhook aktif (cek hanya
	// ke database kita), lebih jarang kalau status harus ditanyakan ke Midtrans.
	PollSeconds int       `json:"poll_seconds"`
	ExpiresAt   time.Time `json:"expires_at"`
	SecondsLeft int       `json:"seconds_left"`
}

type payRow struct {
	id, seq, amount, orderID int
	status                   string
	providerOrderID          sql.NullString
	qrString, qrURL          sql.NullString
	qrExpiresAt              sql.NullTime
}

// loadPayment memuat tagihan dan memastikan tagihan itu milik produk pid.
func (s *PaymentService) loadPayment(pid, payID int) (*payRow, error) {
	var r payRow
	err := s.db.QueryRow(
		"SELECT y.id, y.seq, y.amount, y.order_id, y.status, y.provider_order_id, y.qr_string, y.qr_url, y.qr_expires_at "+
			"FROM payments y JOIN orders o ON o.id = y.order_id WHERE y.id = ? AND o.product_id = ?",
		payID, pid).Scan(&r.id, &r.seq, &r.amount, &r.orderID, &r.status, &r.providerOrderID, &r.qrString, &r.qrURL, &r.qrExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *PaymentService) qrResult(orderID, qr, imageURL string, amount int, exp time.Time) *PayResult {
	left := int(time.Until(exp).Seconds())
	if left < 0 {
		left = 0
	}
	poll := 10
	if s.mt.WebhookEnabled() {
		poll = 3
	}
	return &PayResult{OrderID: orderID, QRString: qr, QRURL: imageURL, Amount: amount, ExpiresAt: exp, SecondsLeft: left, PollSeconds: poll}
}

// Pay mengembalikan QRIS untuk satu tagihan. QR yang sudah ada dan masih berlaku
// dipakai lagi (user menutup lalu membuka aplikasi tetap melihat QR yang sama);
// QR baru dibuat hanya kalau belum ada atau yang lama sudah kedaluwarsa.
func (s *PaymentService) Pay(userID, pid, payID int) (*PayResult, error) {
	r, err := s.loadPayment(pid, payID)
	if err != nil {
		return nil, err
	}
	if r.status == model.PayPaid {
		return nil, NewValidationError("tagihan ini sudah lunas")
	}
	if !s.mt.Enabled() {
		log.Printf("[payment] MIDTRANS_SERVER_KEY kosong: QRIS tidak bisa dibuat")
		return nil, NewValidationError("pembayaran QRIS belum diaktifkan di server, atau bayar cash ke admin")
	}
	// bayar berurutan: tagihan sebelumnya harus lunas dulu
	var before int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM payments WHERE order_id = ? AND seq < ? AND status <> 'paid'",
		r.orderID, r.seq).Scan(&before); err != nil {
		return nil, err
	}
	if before > 0 {
		return nil, NewValidationError("bayar tagihan sebelumnya dulu")
	}

	// QR lama masih berlaku -> tampilkan lagi
	if r.status == model.PayPending && r.providerOrderID.Valid && r.qrString.Valid && r.qrString.String != "" &&
		r.qrExpiresAt.Valid && r.qrExpiresAt.Time.After(time.Now()) {
		return s.qrResult(r.providerOrderID.String, r.qrString.String, r.qrURL.String, r.amount, r.qrExpiresAt.Time), nil
	}

	// QR lama sudah lewat waktu: pastikan belum terbayar di detik terakhir sebelum membuat yang baru
	if r.providerOrderID.Valid {
		if n, err := s.mt.Status(r.providerOrderID.String); err != nil {
			log.Printf("[payment] cek status sebelum QR baru gagal: %v", err)
		} else if err := s.applyStatus(n.OrderID, n.TransactionStatus, n.FraudStatus); err != nil {
			return nil, err
		}
		var st string
		if err := s.db.QueryRow("SELECT status FROM payments WHERE id = ?", r.id).Scan(&st); err != nil {
			return nil, err
		}
		if st == model.PayPaid {
			return nil, NewValidationError("tagihan ini sudah lunas")
		}
	}

	var name, email string
	if err := s.db.QueryRow("SELECT username, email FROM users WHERE id = ?", userID).Scan(&name, &email); err != nil {
		return nil, err
	}
	// order_id harus unik tiap percobaan (transaksi lama yang kedaluwarsa tidak bisa dipakai ulang)
	oid := fmt.Sprintf("SFB%d-%d", r.id, time.Now().Unix())
	q, err := s.mt.CreateQRIS(oid, r.amount, name, email)
	if err != nil {
		log.Printf("[payment] buat QRIS gagal (tagihan %d): %v", r.id, err)
		return nil, NewValidationError("Gagal membuat QRIS: " + err.Error())
	}
	res, err := s.db.Exec(
		"UPDATE payments SET provider_order_id = ?, method = 'qris', status = 'pending', qr_string = ?, qr_url = ?, qr_expires_at = ? "+
			"WHERE id = ? AND status <> 'paid'",
		oid, q.QRString, q.ImageURL, q.ExpiresAt, r.id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, NewValidationError("tagihan ini sudah lunas")
	}
	return s.qrResult(oid, q.QRString, q.ImageURL, r.amount, q.ExpiresAt), nil
}

// Check menanyakan status transaksi ke penyedia QRIS lalu memperbarui tagihan.
// Dipanggil halaman QR secara berkala (webhook tidak bisa menjangkau backend lokal).
func (s *PaymentService) Check(pid, payID int) (string, error) {
	r, err := s.loadPayment(pid, payID)
	if err != nil {
		return "", err
	}
	if r.status == model.PayPaid {
		return model.PayPaid, nil
	}
	if !r.providerOrderID.Valid {
		return r.status, NewValidationError("belum ada transaksi untuk tagihan ini")
	}
	// Webhook aktif: kabar lunas biasanya datang sendiri, jadi cukup baca database.
	// Tetap tanya Midtrans paling sering sekali per 30 detik sebagai cadangan.
	if !s.shouldAsk(r.id) {
		return r.status, nil
	}
	n, err := s.mt.Status(r.providerOrderID.String)
	if err != nil {
		return "", err
	}
	if err := s.applyStatus(n.OrderID, n.TransactionStatus, n.FraudStatus); err != nil {
		return "", err
	}
	var st string
	err = s.db.QueryRow("SELECT status FROM payments WHERE id = ?", r.id).Scan(&st)
	return st, err
}

// HandleNotification memproses webhook Midtrans. Tanda tangan diverifikasi dulu.
func (s *PaymentService) HandleNotification(n payment.Notification) error {
	if !s.mt.VerifySignature(n) {
		return ErrForbidden
	}
	return s.applyStatus(n.OrderID, n.TransactionStatus, n.FraudStatus)
}

// applyStatus menerjemahkan status Midtrans ke status tagihan. Idempoten:
// notifikasi yang sama boleh datang berulang.
func (s *PaymentService) applyStatus(providerOrderID, txStatus, fraud string) error {
	var payID int
	err := s.db.QueryRow("SELECT id FROM payments WHERE provider_order_id = ?", providerOrderID).Scan(&payID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // order_id tidak dikenal (mis. notifikasi tes), abaikan
	}
	if err != nil {
		return err
	}
	switch txStatus {
	case "settlement":
		return s.markPaid(payID, "qris", nil)
	case "capture":
		if fraud == "" || fraud == "accept" {
			return s.markPaid(payID, "qris", nil)
		}
		return nil // challenge: tunggu keputusan
	case "expire":
		return s.setStatus(payID, model.PayExpired)
	case "deny", "cancel", "failure":
		return s.setStatus(payID, model.PayFailed)
	}
	return nil // pending dan lainnya: tidak berubah
}

func (s *PaymentService) setStatus(payID int, status string) error {
	_, err := s.db.Exec("UPDATE payments SET status = ? WHERE id = ? AND status <> 'paid'", status, payID)
	return err
}

// lunas

// ConfirmCash: admin menerima pembayaran cash untuk satu tagihan.
func (s *PaymentService) ConfirmCash(adminID, payID int) error {
	return s.markPaid(payID, "cash", &adminID)
}

// markPaid menandai tagihan lunas (idempoten). DP lunas mengaktifkan produk;
// tagihan lunas juga membuka produk yang ter-suspend kalau tidak ada lagi yang telat.
func (s *PaymentService) markPaid(payID int, method string, adminID *int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var orderID, seq, pid int
	var status string
	err = tx.QueryRow(
		"SELECT y.order_id, y.seq, y.status, o.product_id FROM payments y JOIN orders o ON o.id = y.order_id "+
			"WHERE y.id = ? FOR UPDATE", payID).Scan(&orderID, &seq, &status, &pid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == model.PayPaid {
		return nil
	}
	if _, err := tx.Exec(
		"UPDATE payments SET status = 'paid', paid_at = ?, method = ?, confirmed_by = ? WHERE id = ?",
		time.Now(), method, adminID, payID); err != nil {
		return err
	}
	if seq == 0 {
		if _, err := tx.Exec(
			"UPDATE products SET status = 'active' WHERE id = ? AND status = 'pending_payment'", pid); err != nil {
			return err
		}
	}
	var late int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM payments WHERE order_id = ? AND status <> 'paid' AND due_date < ?",
		orderID, s.overdueCutoff()).Scan(&late); err != nil {
		return err
	}
	if late == 0 {
		if _, err := tx.Exec("UPDATE products SET status = 'active' WHERE id = ? AND status = 'suspended'", pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// cicilan telat

func (s *PaymentService) overdueCutoff() time.Time {
	return today().AddDate(0, 0, -s.cfg.OverdueDays)
}

// SuspendOverdue menyuspend produk aktif yang punya tagihan telat lebih dari OverdueDays.
func (s *PaymentService) SuspendOverdue() (int64, error) {
	// tagihan langganan bulan berjalan harus sudah ada, walau user tidak membuka aplikasi
	rows, err := s.db.Query("SELECT id FROM orders WHERE subscription_fee > 0")
	if err != nil {
		return 0, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.ensureSubscription(id); err != nil {
			log.Printf("[payment] buat tagihan langganan order %d gagal: %v", id, err)
		}
	}

	res, err := s.db.Exec(
		"UPDATE products p JOIN orders o ON o.product_id = p.id SET p.status = 'suspended' "+
			"WHERE p.status = 'active' AND EXISTS ("+
			"SELECT 1 FROM payments y WHERE y.order_id = o.id AND y.status <> 'paid' AND y.due_date < ?)",
		s.overdueCutoff())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RunOverdueJob menjalankan SuspendOverdue sekali di awal lalu tiap jam.
func (s *PaymentService) RunOverdueJob(ctx context.Context) {
	run := func() {
		if n, err := s.SuspendOverdue(); err != nil {
			log.Printf("[payment] cek cicilan telat gagal: %v", err)
		} else if n > 0 {
			log.Printf("[payment] %d produk di-suspend karena cicilan telat", n)
		}
	}
	run()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
