export type RenderContext = Record<string, unknown>;
export type RenderFunction = (context: RenderContext) => string;

export class TemplateRegistry {
  private readonly templates = new Map<string, RenderFunction>();

  add(name: string, render: RenderFunction): void {
    if (!name) throw new Error("template requires name");
    this.templates.set(name, render);
  }

  render(name: string, context: RenderContext = {}): string {
    const render = this.templates.get(name);
    if (!render) throw new Error(`template not found: ${name}`);
    return render(context);
  }
}

export function escape(value: unknown): string {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
