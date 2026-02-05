# Project: TapToSell v2 (Go + React)
**Role:** Sole Developer + AI Assistant
**My Identity:** "Administrator" (Tech Owner / Super Admin)
**Current Phase:** Phase 8 Complete (Polishing & Integration Fixes)

## 🏗 Architecture Context
* **Backend:** Go (Gin), MySQL 8.0.
* **Frontend:** React (Vite, Tailwind).
* **Roles:** `dropshipper`, `supplier`, `manager` (Staff), `administrator` (Me - has Maintenance Mode access).
* **Key Patterns:**
    * **Dual-Database:** Read/Write for App, Read-Only for AI.
    * **Maintenance Mode:** Gated by `administrator` role in `AuthMiddleware`.

## 🚧 Current Code Status (From Audit)
* **Health:** 85/100. Logic is sound, but Data Integration is fragile.
* **Known Conflicts (Fix These First):**
    1.  **JSON Format:** Backend sends Maps (`{Key: Val}`), Frontend expects Arrays.
    2.  **Case Sensitivity:** Backend=`snake_case`, Frontend=`camelCase`.
    3.  **Video Uploads:** Need specific `/upload/video` route.

## 🗺 Roadmap Status
* **Completed:** Phases 1-8 (Core Commerce, AI Chat, Wallet, Inventory).
* **Active Task:** Fixing the "Integration Glue" (The 3 conflicts above).
* **Future (Phase 9):** Webhooks for Shopee/Lazada & Automated Penalty System.