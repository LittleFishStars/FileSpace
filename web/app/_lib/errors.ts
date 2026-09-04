// 错误展示文本工具：统一「Error 取 message、其余回退文案」的提取逻辑，
// 避免各处 catch 里重复写 instanceof 判断。

/** 提取错误展示文本：Error 取 message（空消息同样回退），非 Error 一律回退 */
export function errMsg(e: unknown, fallback: string): string {
    return e instanceof Error && e.message ? e.message : fallback
}
