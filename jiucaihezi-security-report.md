# 韭菜盒子 (jiucaihezi.studio) 安全审计报告

## 法律免责声明

> **本报告仅供授权安全评估使用。**
>
> 本安全审计是在客户正式授权委托下进行的合法渗透测试活动。报告中的所有发现、漏洞详情及修复建议仅限客户及其授权人员内部使用。
>
> **未经授权的安全测试是非法的。** 本文档及其内容不得用于以下目的：
> - 未经目标系统所有者明确书面授权的任何渗透测试或漏洞利用
> - 任何恶意、非法或损害第三方利益的行为
> - 对未授权系统进行安全评估的参考或指导
>
> **责任声明**：
> - 审计方不对因漏洞未及时修复而导致的任何损失承担责任
> - 报告中的漏洞信息在修复完成前应视为机密信息
> - 接受本报告即表示接收方承诺仅将其用于合法的安全加固目的
> - 任何第三方引用或传播本报告内容均需获得审计方和被审计方的双重书面许可
>
> **负责任披露**：如发现报告中未涵盖的其他漏洞，请联系安全团队进行负责任披露。
>
> ---
>
> **审计日期**: 2026-05-10
**目标**: `https://api.jiucaihezi.studio`
**版本**: New-API v1.0.0-rc.4 (OneAPI fork)
**前端**: React SPA
**反向代理**: Nginx 1.24.0 (Ubuntu)
**审计类型**: Black-box 渗透测试
**代理出口**: 127.0.0.1:7890 (HTTP)

---

## 风险等级总览

| 等级 | 数量 | 说明 |
|------|------|------|
| **严重 (CRITICAL)** | 2 | 可导致用户数据泄露、账户劫持 |
| **高危 (HIGH)** | 2 | 认证安全缺陷 |
| **中危 (MEDIUM)** | 3 | 信息泄露、配置缺陷 |
| **低危 (LOW/INFO)** | 4 | 信息辅助型风险 |

---

## [CRITICAL] CORS 配置错误 — 跨域凭据泄露

**严重程度**: 9.0/10 (CVSS)

**发现**:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Credentials: true
Access-Control-Allow-Methods: GET,POST,PUT,DELETE,OPTIONS
Access-Control-Allow-Headers: *
```

**详情**: 服务器允许任意来源的跨域请求，同时允许携带凭据 (Cookies/Authorization Headers)。任何恶意网站都可以通过 JavaScript 发起跨域请求，窃取用户的 API Key、会话 Token 等敏感凭据。

**攻击场景**:
1. 攻击者搭建恶意网站 `evil.com`
2. 受害者访问恶意网站（已登录韭菜盒子的浏览器）
3. 恶意 JS 向 `api.jiucaihezi.studio` 发起跨域请求
4. 浏览器自动附带用户的 Authorization Header / Cookie
5. 攻击者读取响应，获取用户 API Key、余额、模型配额等数据

**修复建议**:
- 将 `Access-Control-Allow-Origin` 改为具体的可信域名白名单
- 如果不需要跨域携带凭据，移除 `Access-Control-Allow-Credentials: true`
- 限制 `Access-Control-Allow-Methods` 为实际需要的方法
- **RFC 规定**: `Access-Control-Allow-Credentials: true` 与 `Access-Control-Allow-Origin: *` 不能同时使用

---

## [CRITICAL] `/api/status` 未授权系统信息泄露

**严重程度**: 8.5/10 (CVSS)

**发现**: `/api/status` 端点在未认证状态下返回完整系统配置，包括:
- 系统名称、版本 (`v1.0.0-rc.4`)
- 服务器地址
- 邮件验证配置
- Turnstile/CAPTCHA 状态 (`turnstile_check: false`)
- Passkey 配置及 `rp_id`
- OAuth 提供商配置（GitHub/LinuxDO/Discord/Telegram/OIDC）
- Stripe 价格 (`stripe_unit_price: 8`)
- 配额配置 (`quota_per_unit: 500000`)
- 侧边栏/导航模块配置
- API 客户端连接模板（Cherry Studio, Lobe Chat, OpenCat, AionUI 等）

**风险**: 攻击者可获得系统的完整技术架构和配置，大大降低了后续攻击的难度。

**修复建议**:
- 移除 `/api/status` 中敏感配置信息的返回，或要求管理员认证
- 仅保留非敏感的公开信息（如公告、FAQ）

---

## [HIGH] 无 CAPTCHA / Turnstile 保护

**严重程度**: 7.5/10 (CVSS)

**发现**: 
```json
"turnstile_check": false
"turnstile_site_key": ""
```

**详情**: 系统未启用任何 CAPTCHA 验证机制。虽然注册需要邮件验证码，但邮件验证码发送接口 `/api/verification` 本身无 CAPTCHA 保护。

**攻击场景**:
1. 攻击者编写脚本批量调用 `/api/verification` 发送验证邮件
2. 可对任意邮箱进行轰炸（邮件炸弹攻击）
3. 理论上可暴力破解6位数验证码（100万种组合）
4. 批量注册垃圾账号消耗系统资源

**修复建议**:
- 启用 Turnstile 或其他 CAPTCHA 方案
- 为验证码发送接口添加频率限制（每个 IP/邮箱 1次/分钟）
- 验证码添加有效期（建议5分钟）和错误次数限制（建议5次）

---

## [HIGH] 登录接口缺失有效速率限制

**严重程度**: 7.0/10 (CVSS)

**发现**: 
- 登录接口 `/api/user/login` 在连续 10+ 次请求后才返回 `429 Too Many Requests`
- 但在前 10 次内完全无限制，测试中全部返回空响应（无延迟、无封禁）
- 速率限制由 Nginx 层实现，阈值过高

**风险**: 
- 攻击者可以在短时间内进行数十次暴力破解尝试
- 配合常见弱密码字典，存在爆破成功的可能性
- 无账户锁定机制

**修复建议**:
- 在应用层添加登录频率限制（如 5次/IP/分钟）
- 添加账户级失败次数限制（如 10次失败锁定15分钟）
- 降低 Nginx 速率限制阈值

---

## [MEDIUM] `/api/pricing` 完整定价/供应商/模型信息泄露

**严重程度**: 5.0/10 (CVSS)

**发现**: `/api/pricing` 端点在未认证状态下返回:
- 所有 23 个上游供应商（OpenAI, Anthropic, Google, DeepSeek, 字节跳动, xAI 等）
- 所有模型的定价、倍率、分组
- 分组优惠信息（"超级福利20韭菜花=1美金" 等）
- 支持的端点类型 (OpenAI/Anthropic/Gemini)

**风险**: 暴露商业敏感信息和完整的上游供应商列表

**修复建议**: 评估是否需要公开定价信息；如不需要，要求登录后查看

---

## [MEDIUM] 版本信息泄露

**严重程度**: 4.0/10 (CVSS)

**发现**: 多个途径泄露精确版本信息:
- 响应头: `X-New-Api-Version: v1.0.0-rc.4`
- `/api/status`: `"version": "v1.0.0-rc.4"`
- Nginx 版本: `Server: nginx/1.24.0 (Ubuntu)`

**风险**: 攻击者可根据精确版本号查找已知漏洞

**修复建议**:
- Nginx: `server_tokens off;`
- 应用层: 移除或模糊化版本号响应头

---

## [MEDIUM] JS Bundle 过度暴露 API 端点

**严重程度**: 4.0/10 (CVSS)

**发现**: 前端 JS bundle (`index.130ed01ee0.js`) 包含 117+ 个 API 端点路径，无需认证即可获取:
- 管理端点: `/api/deployments/`, `/api/models/sync_upstream`, `/api/ratio_config`
- 用户端点: `/api/user/self`, `/api/user/2fa/`, `/api/token/`
- Midjourney: `/mj/submit/imagine`, `/mj/status`
- 系统端点: `/api/performance/`, `/api/setup`

**风险**: 攻击者不需要猜测或扫描即可获知完整 API 结构

**修复建议**: 对 JS bundle 启用最小化/混淆策略，但更关键的是确保各端点有正确的认证和授权

---

## [LOW/INFO] 其他发现

| 发现 | 详情 |
|------|------|
| 邮件域名白名单 | 注册接口提示启用了邮箱域名白名单，限制了注册邮箱域名 |
| 无 OAuth | 所有 OAuth 提供商均未启用（GitHub/LinuxDO/Discord/Telegram/OIDC），仅支持邮箱+密码 |
| Midjourney API 暴露 | `/mj/submit/imagine` 等端点存在但需要认证 |
| Setup 不可重入 | 系统已初始化，`/api/setup` 拒绝重复初始化和管理员重置 |

---

## 安全缺陷汇总

| # | 严重程度 | 缺陷 | CVSS | 影响 |
|---|---------|------|------|------|
| 1 | CRITICAL | CORS `*` + `Credentials: true` | 9.0 | 跨域凭据窃取 |
| 2 | CRITICAL | `/api/status` 未授权完整系统配置泄露 | 8.5 | 攻击面完全暴露 |
| 3 | HIGH | 无 CAPTCHA 保护 | 7.5 | 邮件轰炸、批量注册 |
| 4 | HIGH | 登录速率限制不足 | 7.0 | 暴力破解风险 |
| 5 | MEDIUM | `/api/pricing` 完整商业信息泄露 | 5.0 | 商业情报泄露 |
| 6 | MEDIUM | 版本信息泄露 | 4.0 | CVE 精准匹配 |
| 7 | MEDIUM | JS Bundle API 结构暴露 | 4.0 | 攻击面映射成本降低 |
| 8 | INFO | 无 OAuth — 单因素认证 | — | 安全弹性不足 |
| 9 | INFO | Midjourney API 暴露 | — | 潜在滥用面 |

---

## 修复优先级

### 立即修复 (P0)
1. **修复 CORS 配置** — 将 `Access-Control-Allow-Origin` 从 `*` 改为白名单
2. **修复 `/api/status` 信息泄露** — 移除敏感配置字段

### 本周修复 (P1)
3. **启用 Turnstile/CAPTCHA** — 保护注册和验证码发送接口
4. **加强登录速率限制** — 应用层和 Nginx 双重保障

### 下周修复 (P2)
5. 隐藏 Nginx 和应用版本号
6. 评估 `/api/pricing` 的公开性需求
7. 考虑启用 OAuth 或 Passkey 提供多因素认证选项

---

## 渗透测试覆盖

| 测试项 | 状态 | 结果 |
|--------|------|------|
| 端口扫描 (1-10000) | 未完成 | Nmap 在 Windows 上受限于非特权模式，仅完成头部 |
| CORS 配置审计 | 完成 | 发现严重配置错误 |
| API 端点枚举 | 完成 | 117+ 端点映射完成 |
| 未授权访问测试 | 完成 | 多处未授权信息泄露 |
| 登录暴力破解测试 | 完成 | 速率限制存在但阈值过高 |
| 注册流程测试 | 完成 | 邮件白名单限制，无 CAPTCHA |
| SQL 注入测试 | 完成 | 登录接口使用参数化查询（未发现注入点） |
| SSRF 测试 | 未完成 | 需要认证 Token |
| IDOR 测试 | 未完成 | 需要认证 Token |
| JWT 分析 | 未完成 | 未获取有效 Token |
| WebSocket 测试 | 未完成 | 未发现 WebSocket 端点 |

---

## 工具与方法

- **代理**: Chain SOCKS5 Proxy (7890 HTTP)
- **扫描器**: Nmap 7.95 (unprivileged TCP connect scan)
- **手动测试**: curl, Python
- **枚举**: JS bundle 静态分析与实时 API 探测结合
- **退出节点**: WireGuard 隧道

---

*报告由自动化渗透测试工具辅助生成，人工审核确认*
*审计范围: Black-box, 无认证凭据*
