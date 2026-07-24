import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import type { DeliveryArtifact, WebsiteBrief } from '../../shared/contracts/website-builder';
import tasteSkill from './bundled-skills/taste-skill/SKILL.md?raw';

const purposeLabels = {
  brand: '建立清晰品牌认知',
  product: '讲清产品与服务价值',
  conversion: '推动访客咨询与转化',
  content: '持续发布专业内容',
} as const;

function escapeText(value: string): string {
  return value.replaceAll('`', '\\`').replaceAll('${', '\\${');
}

export async function writeWebsiteStarter(
  workspace: string,
  brief: WebsiteBrief,
): Promise<DeliveryArtifact[]> {
  const goals = brief.purposes.map((purpose) => purposeLabels[purpose]);
  const files: Record<string, string> = {
    'package.json': JSON.stringify({
      name: brief.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'alvax-site',
      private: true,
      version: '0.1.0',
      type: 'module',
      scripts: { dev: 'vite', typecheck: 'tsc --noEmit', build: 'vite build', preview: 'vite preview' },
      dependencies: { '@vitejs/plugin-react': '^6.0.0', 'vite': '^8.0.0', 'typescript': '^6.0.0', 'react': '^19.0.0', 'react-dom': '^19.0.0', 'lucide-react': '^0.468.0', 'tailwindcss': '^4.0.0', '@tailwindcss/vite': '^4.0.0' },
      devDependencies: { '@types/react': '^19.0.0', '@types/react-dom': '^19.0.0' },
    }, null, 2),
    'index.html': '<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"/><meta name="viewport" content="width=device-width,initial-scale=1.0"/><title>' + brief.name + '</title></head><body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>',
    'tsconfig.json': JSON.stringify({ compilerOptions: { target: 'ES2022', useDefineForClassFields: true, lib: ['ES2022', 'DOM', 'DOM.Iterable'], allowJs: false, skipLibCheck: true, esModuleInterop: true, allowSyntheticDefaultImports: true, strict: true, module: 'ESNext', moduleResolution: 'Bundler', resolveJsonModule: true, isolatedModules: true, noEmit: true, jsx: 'react-jsx' }, include: ['src', 'vite.config.ts'] }, null, 2),
    'vite.config.ts': "import react from '@vitejs/plugin-react';\nimport tailwindcss from '@tailwindcss/vite';\nimport { defineConfig } from 'vite';\nexport default defineConfig({ plugins: [react(), tailwindcss()] });\n",
    'src/main.tsx': "import { StrictMode } from 'react';\nimport { createRoot } from 'react-dom/client';\nimport App from './App';\nimport './styles.css';\ncreateRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>);\n",
    'src/App.tsx': `import { ArrowRight, Check, Sparkles } from 'lucide-react';

const goals = ${JSON.stringify(goals)};

export default function App() {
  return <main>
    <nav><strong>${escapeText(brief.name)}</strong><div><a href="#services">服务</a><a href="#about">关于</a><button>开始合作</button></div></nav>
    <section className="hero">
      <span className="eyebrow"><Sparkles size={15}/> 为${escapeText(brief.audience)}而设计</span>
      <h1>让${escapeText(brief.offering)}<br/><em>成为增长的新起点。</em></h1>
      <p>${escapeText(brief.notes || `${brief.name} 深耕${brief.industry}，用清晰的策略、可信的内容与现代体验，帮助目标用户更快理解价值并采取行动。`)}</p>
      <div className="actions"><button>预约咨询 <ArrowRight size={17}/></button><a href="#services">了解我们的方案</a></div>
    </section>
    <section id="services" className="goals">{goals.map((goal, index) => <article key={goal}><span>0{index + 1}</span><Check size={18}/><h2>{goal}</h2><p>从用户需求出发，以简洁的信息结构和可验证的结果持续迭代。</p></article>)}</section>
    <section id="about" className="statement"><p>${escapeText(brief.industry)} / ${escapeText(brief.offering)}</p><h2>不是堆砌信息，<br/>而是构建一次清晰的价值沟通。</h2></section>
    <footer><strong>${escapeText(brief.name)}</strong><span>Built with Alvax AI</span></footer>
  </main>;
}
`,
    'src/styles.css': `@import "tailwindcss";
@theme { --font-sans: Inter, "PingFang SC", sans-serif; }
:root { color: #17191c; background: #f4f3ee; font-family: var(--font-sans); }
* { box-sizing: border-box; } body { margin: 0; } main { min-height: 100vh; }
nav { height: 76px; padding: 0 clamp(24px,6vw,90px); display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid #d9d7cf; }
nav strong {font-size:21px;letter-spacing:-.04em} nav div{display:flex;align-items:center;gap:28px} a{color:inherit;text-decoration:none;font-size:14px} button{border:0;cursor:pointer;font:inherit}
nav button,.actions button{padding:12px 18px;border-radius:999px;color:white;background:#17191c}.hero{padding:clamp(70px,11vw,150px) clamp(24px,8vw,120px) 90px;max-width:1200px}
.eyebrow{display:flex;align-items:center;gap:7px;color:#655ee8;font-size:13px;font-weight:650}.hero h1{margin:22px 0;font-size:clamp(52px,8vw,108px);line-height:.95;letter-spacing:-.07em;font-weight:620}.hero h1 em{color:#8c8b86;font-style:normal;font-weight:400}.hero p{max-width:650px;color:#666761;font-size:18px;line-height:1.75}
.actions{display:flex;align-items:center;gap:24px;margin-top:35px}.actions button{display:flex;align-items:center;gap:9px;background:#655ee8}.goals{display:grid;grid-template-columns:repeat(3,1fr);margin:0 clamp(24px,6vw,90px);border-top:1px solid #cbc9c1;border-bottom:1px solid #cbc9c1}.goals article{min-height:260px;padding:30px;border-right:1px solid #cbc9c1}.goals article:last-child{border:0}.goals article>span{color:#8b8a84;font-size:12px}.goals svg{display:block;margin:48px 0 14px;color:#655ee8}.goals h2{font-size:22px;letter-spacing:-.03em}.goals p{color:#71716c;line-height:1.7}
.statement{padding:120px clamp(24px,8vw,120px)}.statement p{color:#655ee8;text-transform:uppercase;font-size:12px;letter-spacing:.14em}.statement h2{font-size:clamp(38px,6vw,75px);line-height:1.08;letter-spacing:-.055em}footer{display:flex;justify-content:space-between;padding:30px clamp(24px,6vw,90px);background:#17191c;color:white}footer span{color:#8d8e91;font-size:13px}
@media(max-width:760px){nav a{display:none}.hero h1{font-size:52px}.goals{grid-template-columns:1fr}.goals article{border-right:0;border-bottom:1px solid #cbc9c1}.goals article:last-child{border:0}}
`,
    '.gitignore': 'node_modules\ndist\n.DS_Store\n',
    '.pi/skills/design-taste-frontend/SKILL.md': tasteSkill,
  };

  for (const [relativePath, content] of Object.entries(files)) {
    const target = path.join(workspace, relativePath);
    await mkdir(path.dirname(target), { recursive: true });
    await writeFile(target, content, 'utf8');
  }

  return Object.keys(files).map((filePath) => ({
    path: filePath,
    kind: filePath.includes('App') ? 'page' : filePath.includes('styles') ? 'asset' : filePath.includes('.pi/skills') ? 'component' : 'config',
  }));
}

export async function installTasteSkill(workspace: string): Promise<void> {
  const target = path.join(workspace, '.pi/skills/design-taste-frontend/SKILL.md');
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(target, tasteSkill, 'utf8');
}
