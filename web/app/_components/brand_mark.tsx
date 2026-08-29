'use client'

/**
 * 品牌标志：与 filespace-mark.svg 一致的互联网络标识。
 * stroke 用 currentColor 随主题（浅/深）自适应。
 *
 * 顶栏小 logo（装饰性，aria-hidden）与主页大 logo（带语义标签）共用此组件，
 * 仅通过 className 控制尺寸、label 控制可访问性标注：提供 label 时渲染为
 * 带 role="img" + aria-label 的语义元素，否则为纯装饰（aria-hidden）。
 */
export default function BrandMark({className, label}: {className?: string; label?: string}) {
    const mark = (
        <g stroke="currentColor" strokeWidth={14} strokeLinecap="round" fill="none">
            <path d="M214.6 192.3 L358.3 244.9"/>
            <path d="M181.1 225.6 L235.1 373.9"/>
            <path d="M366.9 284.7 L268.8 381.5"/>
            <circle cx="394" cy="258" r="38"/>
            <circle cx="246" cy="404" r="32"/>
            <circle cx="162" cy="173" r="56"/>
            <circle cx="162" cy="173" r="19" strokeWidth={9}/>
        </g>
    );
    if (label) {
        return (
            <svg viewBox="74 85 390 383" className={className} role="img" aria-label={label}>
                {mark}
            </svg>
        );
    }
    return (
        <svg viewBox="74 85 390 383" className={className} aria-hidden>
            {mark}
        </svg>
    );
}
