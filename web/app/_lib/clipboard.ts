// 剪贴板复制工具：统一复制逻辑（Clipboard API 优先，execCommand 降级）。

/**
 * 复制文本到剪贴板。
 * 优先使用 navigator.clipboard（需要安全上下文：https 或 localhost）；
 * 局域网内通过 http://<ip> 直接访问时 Clipboard API 不可用，降级为
 * 临时 textarea + document.execCommand('copy')。
 * 返回是否复制成功。
 */
export async function copyText(text: string): Promise<boolean> {
    try {
        if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(text)
            return true
        }
    } catch {
        // Clipboard API 失败（权限拒绝等）时继续尝试降级方案
    }
    try {
        const ta = document.createElement('textarea')
        ta.value = text
        ta.style.position = 'fixed'
        ta.style.opacity = '0'
        ta.style.pointerEvents = 'none'
        document.body.appendChild(ta)
        ta.focus()
        ta.select()
        const ok = document.execCommand('copy')
        document.body.removeChild(ta)
        return ok
    } catch {
        return false
    }
}