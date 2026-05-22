package routes

import (
	"EverythingSuckz/fsb/internal/bot"
	"EverythingSuckz/fsb/internal/stream"
	"EverythingSuckz/fsb/internal/types"
	"EverythingSuckz/fsb/internal/utils"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gotd/td/tg"
	range_parser "github.com/quantumsheep/range-parser"
	"go.uber.org/zap"
)

func (e *allRoutes) LoadWatch(r *Route) {
	log = e.log.Named("Watch")
	defer log.Info("Loaded watch route")
	r.Engine.GET("/watch/:messageID", getWatchRoute)
	r.Engine.HEAD("/watch/:messageID", getWatchRoute)
}

func getWatchRoute(ctx *gin.Context) {
	w := ctx.Writer
	r := ctx.Request

	messageIDParm := ctx.Param("messageID")
	messageID, err := strconv.Atoi(messageIDParm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	authHash := ctx.Query("hash")
	if authHash == "" {
		http.Error(w, "missing hash param", http.StatusBadRequest)
		return
	}

	// If ?player=1 or Accept header is HTML → serve the watch page
	// Otherwise stream the file directly (so the watch page video src works)
	acceptHeader := r.Header.Get("Accept")
	isPageRequest := ctx.Query("player") == "1" ||
		(acceptHeader != "" && len(acceptHeader) > 4 && acceptHeader[:9] == "text/html")

	worker := bot.GetNextWorker()
	file, err := utils.TimeFuncWithResult(log, "FileFromMessage", func() (*types.File, error) {
		return utils.FileFromMessage(ctx, worker.Client, messageID)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	expectedHash := utils.PackFile(file.FileName, file.FileSize, file.MimeType, file.ID)
	if !utils.CheckHash(authHash, expectedHash) {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}

	if isPageRequest {
		watchURL := fmt.Sprintf("https://melo007-s.hf.space/watch/%d?hash=%s", messageID, authHash)
		streamURL := fmt.Sprintf("/watch/%d?hash=%s", messageID, authHash)
		downloadURL := fmt.Sprintf("/watch/%d?hash=%s&d=true", messageID, authHash)

		mimeType := file.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		isVideo := len(mimeType) >= 5 && mimeType[:5] == "video"
		isAudio := len(mimeType) >= 5 && mimeType[:5] == "audio"
		isImage := len(mimeType) >= 5 && mimeType[:5] == "image"

		html := buildWatchPage(file.FileName, mimeType, streamURL, downloadURL, watchURL, isVideo, isAudio, isImage)
		ctx.Header("Content-Type", "text/html; charset=utf-8")
		ctx.String(http.StatusOK, html)
		return
	}

	// ── Stream the file directly ──────────────────────────────────────────────
	if file.FileSize == 0 {
		res, err := worker.Client.API().UploadGetFile(ctx, &tg.UploadGetFileRequest{
			Location: file.Location,
			Offset:   0,
			Limit:    1024 * 1024,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result, ok := res.(*tg.UploadFile)
		if !ok {
			http.Error(w, "unexpected response", http.StatusInternalServerError)
			return
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", file.FileName))
		if r.Method != "HEAD" {
			ctx.Data(http.StatusOK, file.MimeType, result.GetBytes())
		}
		return
	}

	ctx.Header("Accept-Ranges", "bytes")
	var start, end int64
	rangeHeader := r.Header.Get("Range")

	if ctx.Query("d") == "true" {
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.FileName))
	} else {
		ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", file.FileName))
	}

	if rangeHeader == "" {
		start = 0
		end = file.FileSize - 1
		w.WriteHeader(http.StatusOK)
	} else {
		ranges, err := range_parser.Parse(file.FileSize, rangeHeader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		start = ranges[0].Start
		end = ranges[0].End
		ctx.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, file.FileSize))
		w.WriteHeader(http.StatusPartialContent)
	}

	contentLength := end - start + 1
	mimeType := file.MimeType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	ctx.Header("Content-Type", mimeType)
	ctx.Header("Content-Length", strconv.FormatInt(contentLength, 10))

	if r.Method != "HEAD" {
		pipe, err := stream.NewStreamPipe(ctx, worker.Client, file.Location, start, end, log)
		if err != nil {
			log.Error("Failed to create stream pipe", zap.Error(err))
			return
		}
		defer pipe.Close()
		if _, err := io.CopyN(w, pipe, contentLength); err != nil {
			if !utils.IsClientDisconnectError(err) {
				log.Error("Error while copying stream", zap.Error(err))
			}
		}
	}
}

func buildWatchPage(fileName, mimeType, streamURL, downloadURL, watchURL string, isVideo, isAudio, isImage bool) string {
	var playerBlock string
	if isVideo {
		playerBlock = fmt.Sprintf(`<video controls autoplay preload="metadata" playsinline>
        <source src="%s" type="%s">
      </video>`, streamURL, mimeType)
	} else if isAudio {
		playerBlock = fmt.Sprintf(`<div class="audio-inner">
        <div class="disc" id="disc">🎵</div>
        <audio controls preload="metadata" id="audioEl">
          <source src="%s" type="%s">
        </audio>
      </div>`, streamURL, mimeType)
	} else if isImage {
		playerBlock = fmt.Sprintf(`<img src="%s" alt="%s">`, streamURL, fileName)
	} else {
		playerBlock = fmt.Sprintf(`<div class="no-preview">
        <div style="font-size:4rem">📁</div>
        <p>%s</p>
        <a href="%s" class="btn-dl">↓ Download File</a>
      </div>`, fileName, downloadURL)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d0d0d;color:#f1f1f1;font-family:'Inter',sans-serif;min-height:100vh}
.bar{display:flex;align-items:center;justify-content:space-between;padding:0 20px;height:56px;background:#111;border-bottom:1px solid #222}
.logo{display:flex;align-items:center;gap:10px;text-decoration:none}
.logo-box{width:36px;height:36px;background:linear-gradient(135deg,#7c3aed,#a855f7);border-radius:10px;display:flex;align-items:center;justify-content:center}
.logo-box svg{width:18px;height:18px;fill:#fff}
.logo-name{font-size:17px;font-weight:700;background:linear-gradient(90deg,#a855f7,#7c3aed);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.btn-dl{display:inline-flex;align-items:center;gap:6px;background:#1e1e1e;border:1px solid #333;color:#f1f1f1;padding:8px 18px;border-radius:8px;font-size:13px;font-weight:600;text-decoration:none;transition:background .15s}
.btn-dl:hover{background:#2a2a2a}
.page{max-width:560px;margin:0 auto;padding:20px 16px 60px}
.player-card{background:#111;border-radius:16px;overflow:hidden;margin-bottom:16px}
.player-card video{width:100%%;display:block}
.player-card img{width:100%%;display:block;max-height:400px;object-fit:contain;background:#000}
.audio-inner{padding:40px 24px;display:flex;flex-direction:column;align-items:center;gap:20px;background:#111}
.disc{width:110px;height:110px;border-radius:50%%;background:linear-gradient(135deg,#7c3aed,#a855f7);display:flex;align-items:center;justify-content:center;font-size:44px;box-shadow:0 0 40px rgba(124,58,237,.4);animation:spin 8s linear infinite paused}
.disc.on{animation-play-state:running}
@keyframes spin{to{transform:rotate(360deg)}}
.audio-inner audio{width:100%%;accent-color:#a855f7}
.no-preview{padding:48px 24px;display:flex;flex-direction:column;align-items:center;gap:16px;text-align:center}
.no-preview p{color:#888;font-size:14px;word-break:break-all}
.info-card{background:#111;border-radius:16px;padding:20px;margin-bottom:16px}
.file-name{font-size:17px;font-weight:700;line-height:1.4;word-break:break-word;margin-bottom:16px}
.stats{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:16px}
.stat{background:#1a1a1a;border-radius:10px;padding:12px 14px}
.stat-label{font-size:10px;font-weight:700;color:#555;text-transform:uppercase;letter-spacing:.08em;margin-bottom:3px}
.stat-value{font-size:13px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.action-row{display:flex;gap:10px}
.btn-watch{flex:1;display:flex;align-items:center;justify-content:center;gap:8px;background:linear-gradient(135deg,#7c3aed,#a855f7);color:#fff;border:none;border-radius:10px;padding:13px;font-size:14px;font-weight:700;text-decoration:none;transition:opacity .15s}
.btn-watch:hover{opacity:.9}
.btn-dl2{flex:1;display:flex;align-items:center;justify-content:center;gap:8px;background:#1a1a1a;color:#f1f1f1;border:1px solid #333;border-radius:10px;padding:13px;font-size:14px;font-weight:700;text-decoration:none;transition:background .15s}
.btn-dl2:hover{background:#222}
.share-card{background:#111;border-radius:16px;padding:20px}
.share-head{font-size:13px;font-weight:700;color:#888;text-transform:uppercase;letter-spacing:.08em;margin-bottom:14px}
.link-box{display:flex;gap:8px;margin-bottom:10px}
.link-input{flex:1;background:#1a1a1a;border:1px solid #2a2a2a;border-radius:8px;padding:10px 12px;color:#f1f1f1;font-size:12px;font-family:monospace;outline:none;min-width:0}
.link-input:focus{border-color:#7c3aed}
.copy-btn{background:#1a1a1a;border:1px solid #333;color:#f1f1f1;border-radius:8px;padding:10px 16px;font-size:13px;font-weight:600;cursor:pointer;font-family:inherit;white-space:nowrap}
.copy-btn:hover{background:#2a2a2a}
.social-row{display:flex;gap:8px;margin-top:12px}
.social-btn{flex:1;display:flex;align-items:center;justify-content:center;gap:6px;background:#1a1a1a;border:1px solid #2a2a2a;border-radius:8px;padding:10px;font-size:13px;font-weight:600;color:#f1f1f1;text-decoration:none}
.social-btn:hover{background:#222}
.toast{position:fixed;bottom:28px;left:50%%;transform:translateX(-50%%) translateY(80px);background:#1e1e1e;border:1px solid #333;border-radius:10px;padding:10px 22px;font-size:14px;font-weight:600;z-index:999;transition:transform .3s cubic-bezier(.34,1.56,.64,1);white-space:nowrap}
.toast.show{transform:translateX(-50%%) translateY(0)}
</style>
</head>
<body>
<div class="bar">
  <a href="/" class="logo">
    <div class="logo-box"><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg></div>
    <span class="logo-name">TeleStream</span>
  </a>
  <a href="%s" class="btn-dl">↓ Download</a>
</div>
<div class="page">
  <div class="player-card">%s</div>
  <div class="info-card">
    <div class="file-name">%s</div>
    <div class="stats">
      <div class="stat"><div class="stat-label">Type</div><div class="stat-value">%s</div></div>
      <div class="stat"><div class="stat-label">File</div><div class="stat-value" title="%s">%s</div></div>
    </div>
    <div class="action-row">
      <a href="%s" class="btn-watch">▶ Stream</a>
      <a href="%s" class="btn-dl2">↓ Download</a>
    </div>
  </div>
  <div class="share-card">
    <div class="share-head">Share this file</div>
    <div class="link-box">
      <input class="link-input" id="sl" type="text" value="%s" readonly onclick="this.select()">
      <button class="copy-btn" onclick="cp()">Copy</button>
    </div>
    <div class="social-row">
      <a class="social-btn" href="https://t.me/share/url?url=%s" target="_blank">📨 Telegram</a>
      <a class="social-btn" href="https://wa.me/?text=%s" target="_blank">💬 WhatsApp</a>
    </div>
  </div>
</div>
<div class="toast" id="toast"></div>
<script>
function cp(){const el=document.getElementById('sl');el.select();navigator.clipboard.writeText(el.value).then(()=>{const t=document.getElementById('toast');t.textContent='✅ Link copied!';t.classList.add('show');setTimeout(()=>t.classList.remove('show'),2200)})}
const a=document.getElementById('audioEl'),d=document.getElementById('disc');
if(a&&d){a.addEventListener('play',()=>d.classList.add('on'));a.addEventListener('pause',()=>d.classList.remove('on'))}
</script>
</body>
</html>`,
		fileName,
		downloadURL,
		playerBlock,
		fileName,
		mimeType, fileName, fileName,
		streamURL, downloadURL,
		watchURL,
		watchURL,
		fileName+"%0A"+watchURL,
	)
}
