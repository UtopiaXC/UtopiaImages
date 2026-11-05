package mailer

import (
	"fmt"
	"html"
)

// BuildEmailHTML wraps the content into a responsive, modern dark-themed HTML email layout.
func BuildEmailHTML(siteName, title, username, messageHTML, buttonText, buttonURL, tokenText, noteText string) string {
	if siteName == "" {
		siteName = "Utopia Images"
	}
	escapedSiteName := html.EscapeString(siteName)
	escapedTitle := html.EscapeString(title)
	escapedUsername := html.EscapeString(username)

	var actionButtonHTML string
	if buttonText != "" && buttonURL != "" {
		actionButtonHTML = fmt.Sprintf(`
			<div style="text-align: center; margin: 28px 0 20px;">
				<a href="%s" target="_blank" rel="noopener noreferrer" style="display: inline-block; background: linear-gradient(135deg, #63e2b7 0%%, #42b883 100%%); color: #0d0d12; font-weight: 600; font-size: 15px; text-decoration: none; padding: 13px 32px; border-radius: 8px; box-shadow: 0 4px 14px rgba(99, 226, 183, 0.35); letter-spacing: 0.3px;">
					%s
				</a>
			</div>
		`, html.EscapeString(buttonURL), html.EscapeString(buttonText))
	}

	var tokenBoxHTML string
	if tokenText != "" {
		tokenBoxHTML = fmt.Sprintf(`
			<div style="margin: 20px 0; padding: 14px 18px; background: rgba(99, 226, 183, 0.08); border: 1px dashed rgba(99, 226, 183, 0.5); border-radius: 8px; text-align: center;">
				<div style="font-size: 12px; color: #888; margin-bottom: 6px; text-transform: uppercase; letter-spacing: 1px;">重置令牌 (Token)</div>
				<div style="font-family: 'Consolas', 'Courier New', Courier, monospace; font-size: 20px; font-weight: 700; color: #63e2b7; letter-spacing: 2px; word-break: break-all;">
					%s
				</div>
			</div>
		`, html.EscapeString(tokenText))
	}

	var fallbackLinkHTML string
	if buttonURL != "" {
		fallbackLinkHTML = fmt.Sprintf(`
			<div style="margin-top: 24px; padding-top: 18px; border-top: 1px solid rgba(255, 255, 255, 0.06); font-size: 12px; color: #777; line-height: 1.6;">
				<span>若上方按钮无法直接点击，请复制以下链接到浏览器地址栏访问：</span><br />
				<a href="%s" style="color: #63e2b7; text-decoration: underline; word-break: break-all;">%s</a>
			</div>
		`, html.EscapeString(buttonURL), html.EscapeString(buttonURL))
	}

	var noteHTML string
	if noteText != "" {
		noteHTML = fmt.Sprintf(`
			<div style="margin-top: 16px; padding: 10px 14px; background: rgba(255, 255, 255, 0.02); border-left: 3px solid #f2c97d; font-size: 12px; color: #aaa; border-radius: 0 6px 6px 0; line-height: 1.5;">
				%s
			</div>
		`, html.EscapeString(noteText))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>%s - %s</title>
</head>
<body style="margin: 0; padding: 0; background-color: #101014; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; -webkit-font-smoothing: antialiased; color: #e4e4e7;">
	<table role="presentation" width="100%%" border="0" cellspacing="0" cellpadding="0" style="background-color: #101014; min-height: 100vh; padding: 36px 16px;">
		<tr>
			<td align="center" valign="top">
				<table role="presentation" width="100%%" border="0" cellspacing="0" cellpadding="0" style="max-width: 560px; background-color: #18181c; border-radius: 14px; border: 1px solid rgba(255, 255, 255, 0.08); box-shadow: 0 16px 40px rgba(0, 0, 0, 0.6); overflow: hidden;">
					<!-- Brand Header -->
					<tr>
						<td style="padding: 24px 28px 20px; border-bottom: 1px solid rgba(255, 255, 255, 0.06); background: rgba(255, 255, 255, 0.01);">
							<table role="presentation" width="100%%" border="0" cellspacing="0" cellpadding="0">
								<tr>
									<td style="vertical-align: middle;">
										<div style="display: inline-block; width: 32px; height: 32px; line-height: 32px; background: #63e2b7; color: #0d0d12; font-weight: 800; font-size: 14px; text-align: center; border-radius: 8px; vertical-align: middle; margin-right: 10px;">UI</div>
										<span style="font-size: 17px; font-weight: 700; color: #63e2b7; vertical-align: middle; letter-spacing: 0.5px;">%s</span>
									</td>
								</tr>
							</table>
						</td>
					</tr>
					<!-- Main Content -->
					<tr>
						<td style="padding: 32px 32px 28px;">
							<h1 style="margin: 0 0 16px; font-size: 19px; font-weight: 600; color: #ffffff; letter-spacing: 0.2px;">%s</h1>
							<p style="margin: 0 0 16px; font-size: 14px; color: #d4d4d8; line-height: 1.6;">
								您好 <strong style="color: #63e2b7;">%s</strong>：
							</p>
							<div style="font-size: 14px; color: #d4d4d8; line-height: 1.6;">
								%s
							</div>
							%s
							%s
							%s
							%s
						</td>
					</tr>
					<!-- Footer -->
					<tr>
						<td style="padding: 18px 28px 24px; border-top: 1px solid rgba(255, 255, 255, 0.06); text-align: center; font-size: 12px; color: #71717a; background: rgba(0, 0, 0, 0.15);">
							<div>本邮件由系统自动发出，请勿直接回复。</div>
							<div style="margin-top: 6px; color: #52525b;">
								Powered by <strong style="color: #a1a1aa;">%s</strong> · Next-Generation Image Service
							</div>
						</td>
					</tr>
				</table>
			</td>
		</tr>
	</table>
</body>
</html>`,
		escapedTitle, escapedSiteName,
		escapedSiteName,
		escapedTitle,
		escapedUsername,
		messageHTML,
		tokenBoxHTML,
		actionButtonHTML,
		fallbackLinkHTML,
		noteHTML,
		escapedSiteName,
	)
}

// RenderPasswordResetEmail generates HTML and plain-text email for password reset.
func RenderPasswordResetEmail(siteName, username, resetLink string) (subject, htmlBody, textBody string) {
	if siteName == "" {
		siteName = "Utopia Images"
	}
	subject = fmt.Sprintf("[%s] 密码重置请求", siteName)
	messageHTML := `<p style="margin: 0 0 12px;">我们收到了您的账号密码重置请求。请在 <strong>15 分钟内</strong> 点击下方按钮重置您的密码：</p>`
	noteText := "若您未发起此重置请求，请忽略本邮件，您的账号原密码依然安全有效。"

	htmlBody = BuildEmailHTML(siteName, "密码重置请求", username, messageHTML, "重置我的密码", resetLink, "", noteText)
	textBody = fmt.Sprintf("您好 %s：\r\n\r\n我们收到了您的密码重置请求。请在 15 分钟内点击以下链接重置密码：\r\n%s\r\n\r\n若您未发起此重置请求，请忽略本邮件，原账号密码依然安全有效。\r\n-- %s", username, resetLink, siteName)
	return
}

// Render2FAResetEmail generates HTML and plain-text email for 2FA reset.
func Render2FAResetEmail(siteName, username, resetLink, token string) (subject, htmlBody, textBody string) {
	if siteName == "" {
		siteName = "Utopia Images"
	}
	subject = fmt.Sprintf("[%s] 两步验证 (2FA) 重置请求", siteName)
	messageHTML := `<p style="margin: 0 0 12px;">我们收到了您的两步验证 (2FA) 重置申请。请在 <strong>15 分钟内</strong> 点击下方按钮重置 2FA，或在页面中手动输入下方的重置令牌：</p>`
	noteText := "重置完成后两步验证将被解除，您可以使用账号密码直接登录。若您未发起此重置请求，请立即忽略本邮件并检查账号安全。"

	htmlBody = BuildEmailHTML(siteName, "两步验证 (2FA) 重置请求", username, messageHTML, "前往重置两步验证", resetLink, token, noteText)
	textBody = fmt.Sprintf("您好 %s：\r\n\r\n我们收到了您的两步验证 (2FA) 重置请求。请在 15 分钟内点击以下链接重置 2FA，或在页面输入重置令牌：\r\n重置链接：%s\r\n重置令牌：%s\r\n\r\n重置后两步验证将被解除，您可以使用账号密码直接登录系统。\r\n若您未发起此重置请求，请忽略本邮件。\r\n-- %s", username, resetLink, token, siteName)
	return
}

// RenderActivationEmail generates HTML and plain-text email for account activation.
func RenderActivationEmail(siteName, username, activationLink string) (subject, htmlBody, textBody string) {
	if siteName == "" {
		siteName = "Utopia Images"
	}
	subject = fmt.Sprintf("[%s] 账号激活确认", siteName)
	messageHTML := fmt.Sprintf(`<p style="margin: 0 0 12px;">欢迎注册 <strong>%s</strong>！</p><p style="margin: 0 0 12px;">您的账号已成功创建，目前处于待激活状态。请在 <strong>24 小时内</strong> 点击下方按钮完成邮箱验证并激活账号：</p>`, html.EscapeString(siteName))
	noteText := "激活完成后，您即可正常登录并使用平台。若您未注册过此账号，请忽略此邮件。"

	htmlBody = BuildEmailHTML(siteName, "账号激活确认", username, messageHTML, "立即激活账号", activationLink, "", noteText)
	textBody = fmt.Sprintf("您好 %s：\r\n\r\n欢迎注册 %s！\r\n您的账号已成功创建，目前处于待激活状态。请在 24 小时内点击下方链接完成邮箱激活：\r\n%s\r\n\r\n激活完成后，您即可正常登录并使用 %s 平台。若您未注册过此账号，请忽略此邮件。\r\n-- %s", username, siteName, activationLink, siteName, siteName)
	return
}
