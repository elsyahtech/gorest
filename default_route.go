package gorest

import (
	"github.com/gofiber/fiber/v3"
)

func RegisterDefaultRoutes(app *App) {
	app.Get("/", func(ctx *Context) error {
		htmlContent := `
        <!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <title>GOREST</title>

            <style>
                * {
                    box-sizing: border-box;
                }

                body {
                    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI",
                        Roboto, Helvetica, Arial, sans-serif;

                    color: #f8fafc;
                    display: flex;
                    justify-content: center;
                    align-items: center;

                    min-height: 100vh;
                    margin: 0;
                    overflow: hidden;

                    background: #020617;
                }

                /* ==========================================================
                   BACKGROUND
                   ========================================================== */

                .background {
                    position: fixed;
                    inset: 0;

                    width: 100%;
                    height: 100%;

                    object-fit: cover;

                    z-index: 0;
                }

                /* Dark overlay to keep text readable */
                .overlay {
                    position: fixed;
                    inset: 0;

                    background: rgba(2, 6, 23, 0.30);

                    z-index: 1;
                }

                /* ==========================================================
                   WELCOME CONTENT
                   ========================================================== */

                .container {
                    position: relative;
                    z-index: 2;

                    text-align: center;

                    padding: 40px 50px;

                    background: rgba(15, 23, 42, 0.78);

                    border: 1px solid rgba(148, 163, 184, 0.15);

                    border-radius: 12px;

                    box-shadow:
                        0 10px 25px rgba(0, 0, 0, 0.35),
                        0 0 40px rgba(14, 165, 233, 0.08);

                    backdrop-filter: blur(6px);
                    -webkit-backdrop-filter: blur(6px);
                }

                h1 {
                    color: #38bdf8;
                    margin: 0 0 10px;
                }

                p {
                    color: #94a3b8;
                    margin: 0;
                }
                
                a {
                    color: #ecf0f4
                }
            </style>
        </head>

        <body>

            <!--
                Gorest default background.

                Place the image at:
                    ./server/gorest.png
            -->
            <img
                src="data:image/png;base64,` + gorestBackgroundBase64 + `"
                alt="Gorest"
                class="background"
            >

            <div class="overlay"></div>

            <div class="container">
                <h1>Welcome to Gorest! 🦈</h1>
                <p>&nbsp;</p>
                <p>
                    <a href="https://github.com/elsyahtech/gorest">Github</a>
                </p>
            </div>

        </body>
        </html>
        `

		ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)

		return ctx.SendString(htmlContent)
	})
}
