package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// GET /api/user/sessions — the user's currently online devices: only sessions
// whose login token hasn't expired, one per device (deduped by ip+user_agent).
// Dead (token-expired) rows are purged so the count reflects who's online now.
func (a *API) handleUserSessions(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(ctxUserID).(int64)
	jti, _ := r.Context().Value(ctxJti).(string)
	minCreated := time.Now().Unix() - int64(tokenTTL/time.Second)
	_, _ = a.st.PurgeExpiredSessions(minCreated)
	list, err := a.st.ListActiveSessions(uid, minCreated, jti)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取登录设备失败")
		return
	}
	for _, s := range list {
		s.Current = s.Jti == jti
	}
	ok(w, list)
}

// POST /api/user/sessions/{id}/revoke — log out a specific device.
func (a *API) handleUserRevokeSession(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(ctxUserID).(int64)
	id := atoi(chi.URLParam(r, "id"))
	if id <= 0 {
		fail(w, http.StatusBadRequest, "无效的会话 id")
		return
	}
	if err := a.st.DeleteUserSession(uid, id); err != nil {
		fail(w, http.StatusInternalServerError, "注销失败")
		return
	}
	ok(w, nil)
}

// handleAdminSessions lists every user's login sessions (the admin global device
// view). Online is marked by token-TTL: a session whose token hasn't expired is
// "当前在线", otherwise it's historical ("登录过").
func (a *API) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	list, err := a.st.ListAllSessions()
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取登录设备失败")
		return
	}
	minCreated := time.Now().Unix() - int64(tokenTTL/time.Second)
	for i := range list {
		list[i].Online = list[i].CreatedAt >= minCreated
	}
	ok(w, list)
}

// handleAdminRevokeSession force-logs out a session of any user (admin action).
func (a *API) handleAdminRevokeSession(w http.ResponseWriter, r *http.Request) {
	id := atoi(chi.URLParam(r, "id"))
	if id <= 0 {
		fail(w, http.StatusBadRequest, "无效的会话 id")
		return
	}
	if err := a.st.DeleteSessionByID(id); err != nil {
		fail(w, http.StatusInternalServerError, "注销失败")
		return
	}
	ok(w, nil)
}

// handleAdminProtocolUsage returns per-account per-protocol cumulative traffic
// (e.g. vless / hysteria2). user_id is optional; omitted = all users.
func (a *API) handleAdminProtocolUsage(w http.ResponseWriter, r *http.Request) {
	var uid int64
	if v := r.URL.Query().Get("user_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			uid = n
		}
	}
	list, err := a.st.ListProtocolUsage(uid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取协议用量失败")
		return
	}
	ok(w, list)
}
