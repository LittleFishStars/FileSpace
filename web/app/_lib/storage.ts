// localStorage 安全访问封装：隐私模式 / 存储被禁用 / 配额超限等场景下
// localStorage 读写会抛异常，统一吞掉并回退（读取返回 null、写入忽略），
// 避免页面功能因存储不可用而崩溃（数据降级为仅当前会话内生效）。
// 服务端渲染（无 window）时读写同样安全返回，无需调用方额外判断。

function localStorageSafe(): Storage | null {
    return typeof window === 'undefined' ? null : window.localStorage
}

/** 读取键值；存储不可用或键不存在时返回 null */
export function storageGet(key: string): string | null {
    try {
        return localStorageSafe()?.getItem(key) ?? null
    } catch {
        return null
    }
}

/** 写入键值；存储不可用时忽略（不影响本次会话内生效） */
export function storageSet(key: string, value: string): void {
    try {
        localStorageSafe()?.setItem(key, value)
    } catch {
        // 忽略：无持久化能力时降级为仅内存会话
    }
}

/** 删除键值；存储不可用时忽略 */
export function storageRemove(key: string): void {
    try {
        localStorageSafe()?.removeItem(key)
    } catch {
        // 忽略
    }
}
