// 修补 @ant-design/pro-layout 内部 Drawer 的废弃 width 属性。
// antd v6 中 Drawer 的 width 已废弃（改用 size，数值等价），
// ProLayout 在小屏时渲染 <Drawer width={siderWidth}> 触发控制台警告。
// 该脚本在 pnpm install 后通过 postinstall 自动运行，幂等。
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pnpmDir = path.join(root, 'node_modules', '.pnpm');

let patched = 0;

if (fs.existsSync(pnpmDir)) {
  for (const entry of fs.readdirSync(pnpmDir)) {
    if (!entry.startsWith('@ant-design+pro-layout@')) continue;
    const file = path.join(
      pnpmDir,
      entry,
      'node_modules',
      '@ant-design',
      'pro-layout',
      'es',
      'components',
      'SiderMenu',
      'index.js',
    );
    if (!fs.existsSync(file)) continue;
    const src = fs.readFileSync(file, 'utf8');
    const next = src.replace(/width:\s*siderWidth/, 'size: siderWidth');
    if (next !== src) {
      fs.writeFileSync(file, next);
      console.log(`[patch-pro-layout] ${path.relative(root, file)}`);
      patched++;
    }
  }
}

if (patched > 0) {
  console.log(`✅ pro-layout 内部 Drawer width→size 已修补（${patched} 处）`);
} else {
  console.log('pro-layout 无需修补或已修补');
}
