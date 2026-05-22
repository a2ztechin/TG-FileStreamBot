package routes

import (
	"EverythingSuckz/fsb/internal/bot"
	"EverythingSuckz/fsb/internal/utils"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (e *allRoutes) LoadWatch(r *Route) {
	log = e.log.Named("Watch")
	defer log.Info("Loaded watch route")
	r.Engine.GET("/watch/:messageID", getWatchRoute)
}

func getWatchRoute(ctx *gin.Context) {
	messageIDParm := ctx.Param("messageID")
	messageID, err := strconv.Atoi(messageIDParm)
	if err != nil {
		http.Error(ctx.Writer, "invalid message ID", http.StatusBadRequest)
		return
	}

	authHash := ctx.Query("hash")
	if authHash == "" {
		http.Error(ctx.Writer, "missing hash param", http.StatusBadRequest)
		return
	}

	worker := bot.GetNextWorker()
	file, err := utils.FileFromMessage(ctx, worker.Client, messageID)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return
	}

	streamURL := fmt.Sprintf("/stream/%d?hash=%s", messageID, authHash)
	downloadURL := fmt.Sprintf("/stream/%d?hash=%s&d=true", messageID, authHash)

	mimeType := file.MimeType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	isVideo := len(mimeType) >= 5 && mimeType[:5] == "video"
	isAudio := len(mimeType) >= 5 && mimeType[:5] == "audio"
	isImage := len(mimeType) >= 5 && mimeType[:5] == "image"

	html := buildWatchPage(file.FileName, mimeType, streamURL, downloadURL, isVideo, isAudio, isImage)
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.String(http.StatusOK, html)
}

func buildWatchPage(fileName, mimeType, streamURL, downloadURL string, isVideo, isAudio, isImage bool) string {
	var playerBlock string
	if isVideo {
		playerBlock = fmt.Sprintf(`<div class="player-box">
      <video controls autoplay preload="metadata" playsinline>
        <source src="%s" type="%s">
        <a href="%s">Download file</a>
      </video>
    </div>`, streamURL, mimeType, downloadURL)
	} else if isAudio {
		playerBlock = fmt.Sprintf(`<div class="player-box audio-box">
      <div class="audio-disc" id="disc">🎵</div>
      <div class="audio-name">%s</div>
      <audio controls preload="metadata" class="audio-ctrl">
        <source src="%s" type="%s">
      </audio>
    </div>`, fileName, streamURL, mimeType)
	} else if isImage {
		playerBlock = fmt.Sprintf(`<div class="player-box img-box">
      <img src="%s" alt="%s" loading="lazy">
    </div>`, streamURL, fileName)
	} else {
		playerBlock = fmt.Sprintf(`<div class="player-box doc-box">
      <div style="font-size:5rem">📁</div>
      <h2 style="margin:1rem 0 0.5rem;font-size:1.1rem">%s</h2>
      <p style="color:var(--text2);margin-bottom:1.5rem">This file type cannot be previewed.</p>
      <a href="%s" class="btn btn-red">↓ Download File</a>
    </div>`, fileName, downloadURL)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s — TeleStream</title>
  <meta property="og:title" content="%s">
  <meta property="og:type" content="video.other">
  <meta property="og:video" content="%s">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    *,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
    :root{
      --bg:#0f0f0f;--bg2:#1a1a1a;--bg3:#272727;
      --border:#3d3d3d;--text:#f1f1f1;--text2:#aaaaaa;--text3:#717171;
      --red:#ff0000;--red2:#cc0000;--r:12px;
    }
    body{background:var(--bg);color:var(--text);font-family:'Inter',sans-serif;font-size:14px;min-height:100vh}

    /* Topbar */
    .topbar{position:sticky;top:0;z-index:100;background:var(--bg);border-bottom:1px solid var(--border);display:flex;align-items:center;justify-content:space-between;padding:0 16px;height:56px}
    .logo{display:flex;align-items:center;gap:8px;text-decoration:none}
    .logo-icon{width:34px;height:24px;background:var(--red);border-radius:6px;display:flex;align-items:center;justify-content:center;flex-shrink:0}
    .logo-icon svg{width:16px;height:16px;fill:#fff}
    .logo-text{font-size:18px;font-weight:700;color:var(--text)}

    /* Layout — single column on mobile, two-col on desktop */
    .layout{max-width:900px;margin:0 auto;padding:16px 16px 48px}

    /* Player */
    .player-box{background:#000;border-radius:var(--r);overflow:hidden;width:100%%;aspect-ratio:16/9;display:flex;align-items:center;justify-content:center}
    .player-box video{width:100%%;height:100%%;display:block}
    .player-box img{max-width:100%%;max-height:100%%;object-fit:contain}
    .audio-box{aspect-ratio:unset;padding:40px 24px;flex-direction:column;gap:16px;background:var(--bg2)}
    .img-box{aspect-ratio:unset;padding:16px;min-height:300px;background:var(--bg2)}
    .doc-box{aspect-ratio:unset;padding:4rem 2rem;flex-direction:column;text-align:center;background:var(--bg2)}
    .audio-disc{width:100px;height:100px;border-radius:50%%;background:linear-gradient(135deg,#ff0000,#ff6b6b);display:flex;align-items:center;justify-content:center;font-size:40px;box-shadow:0 8px 32px rgba(255,0,0,.3);animation:spin 8s linear infinite paused}
    .audio-disc.playing{animation-play-state:running}
    @keyframes spin{to{transform:rotate(360deg)}}
    .audio-name{font-size:14px;font-weight:600;text-align:center;max-width:100%%;word-break:break-word}
    .audio-ctrl{width:100%%;max-width:480px;height:48px;accent-color:var(--red)}

    /* Info section */
    .info{padding:14px 0 0}
    .vtitle{font-size:18px;font-weight:700;line-height:1.4;margin-bottom:12px;word-break:break-word}
    
    .meta-row{display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:10px;padding-bottom:14px;border-bottom:1px solid var(--border);margin-bottom:14px}
    .meta-stats{color:var(--text2);font-size:13px;display:flex;gap:6px;flex-wrap:wrap;align-items:center}
    .dot{color:var(--text3)}
    .actions{display:flex;gap:8px;flex-wrap:wrap;flex-shrink:0}

    .btn{display:inline-flex;align-items:center;gap:6px;padding:8px 16px;border-radius:20px;font-size:14px;font-weight:600;font-family:inherit;cursor:pointer;border:none;transition:background .15s;text-decoration:none;white-space:nowrap}
    .btn-red{background:var(--red);color:#fff}.btn-red:hover{background:var(--red2)}
    .btn-dark{background:var(--bg3);color:var(--text);border:1px solid var(--border)}.btn-dark:hover{background:#333}

    .ch-row{display:flex;align-items:center;gap:12px;padding-bottom:14px;border-bottom:1px solid var(--border);margin-bottom:14px}
    .ch-avatar{width:40px;height:40px;border-radius:50%%;background:linear-gradient(135deg,var(--red),#ff6b6b);display:flex;align-items:center;justify-content:center;font-weight:700;font-size:16px;flex-shrink:0}

    /* Details grid */
    .details{display:grid;grid-template-columns:repeat(2,1fr);gap:10px;margin-bottom:16px}
    .dc{background:var(--bg2);border:1px solid var(--border);border-radius:10px;padding:12px 14px}
    .dl{font-size:11px;font-weight:600;color:var(--text3);text-transform:uppercase;letter-spacing:.08em;margin-bottom:4px}
    .dv{font-size:14px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

    /* Share box — at bottom */
    .share-box{background:var(--bg2);border:1px solid var(--border);border-radius:var(--r);padding:16px}
    .share-title{font-weight:600;margin-bottom:12px;font-size:15px}
    .link-row{display:flex;gap:8px;margin-bottom:8px}
    .link-input{flex:1;background:var(--bg3);border:1px solid var(--border);border-radius:8px;padding:8px 12px;color:var(--text);font-size:12px;font-family:monospace;outline:none;min-width:0}
    .link-input:focus{border-color:var(--red)}
    .share-btns{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}

    /* Toast */
    .toast{position:fixed;bottom:24px;left:50%%;transform:translateX(-50%%) translateY(80px);background:var(--bg3);border:1px solid var(--border);border-radius:8px;padding:10px 20px;font-size:14px;font-weight:600;z-index:9999;transition:transform .25s cubic-bezier(.34,1.56,.64,1);white-space:nowrap}
    .toast.show{transform:translateX(-50%%) translateY(0)}

    @media(max-width:480px){
      .vtitle{font-size:15px}
      .btn{padding:7px 12px;font-size:13px}
      .details{grid-template-columns:1fr 1fr}
    }
  </style>
</head>
<body>

<header class="topbar">
  <a href="/" class="logo">
    <div class="logo-icon"><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg></div>
    <span class="logo-text">TeleStream</span>
  </a>
  <a href="%s" class="btn btn-dark">↓ Download</a>
</header>

<div class="layout">

  %s

  <div class="info">
    <h1 class="vtitle">%s</h1>

    <div class="meta-row">
      <div class="meta-stats">
        <span>%s</span>
        <span class="dot">•</span>
        <span>%s</span>
      </div>
      <div class="actions">
        <a href="%s" class="btn btn-red">↓ Download</a>
      </div>
    </div>

    <div class="ch-row">
      <div class="ch-avatar">T</div>
      <div>
        <div style="font-weight:600;font-size:15px">TeleStream</div>
        <div style="font-size:12px;color:var(--text3)">Instant Telegram Streaming</div>
      </div>
    </div>

    <div class="details">
      <div class="dc"><div class="dl">Type</div><div class="dv">%s</div></div>
      <div class="dc"><div class="dl">File Name</div><div class="dv" title="%s">%s</div></div>
    </div>

    <div class="share-box">
      <div class="share-title">🔗 Share this file</div>
      <div class="link-row">
        <input type="text" class="link-input" id="wlink" value="%s" readonly onclick="this.select()">
        <button class="btn btn-dark" onclick="copyText('wlink','Link copied!')">Copy</button>
      </div>
      <div class="share-btns">
        <a href="https://t.me/share/url?url=%s" target="_blank" class="btn btn-dark">📨 Telegram</a>
        <a href="https://wa.me/?text=%s" target="_blank" class="btn btn-dark">💬 WhatsApp</a>
      </div>
    </div>

  </div>
</div>

<div class="toast" id="toast"></div>
<script>
  function copyText(id,msg){const el=document.getElementById(id);el.select();navigator.clipboard.writeText(el.value).then(()=>showToast('✅ '+msg))}
  function showToast(msg){const t=document.getElementById('toast');t.textContent=msg;t.classList.add('show');setTimeout(()=>t.classList.remove('show'),2500)}
  const audio=document.querySelector('audio');
  const disc=document.getElementById('disc');
  if(audio&&disc){audio.addEventListener('play',()=>disc.classList.add('playing'));audio.addEventListener('pause',()=>disc.classList.remove('playing'))}
</script>
</body>
</html>`,
		fileName, fileName, streamURL,
		downloadURL,
		playerBlock,
		fileName,
		mimeType, mimeType,
		downloadURL,
		mimeType, fileName, fileName,
		streamURL,
		streamURL,
		fileName,
	)
}
