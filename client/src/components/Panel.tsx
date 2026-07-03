import type { JSX } from "solid-js";

interface PanelProps {
  title: string;
  collapsed: boolean;
  summary?: JSX.Element;
  actions?: JSX.Element;
  contentClass?: string;
  children: JSX.Element;
  onToggle: () => void;
}

export function Panel(props: PanelProps) {
  return (
    <section classList={{ panel: true, collapsed: props.collapsed }}>
      <div class="panel-title">
        <button
          type="button"
          class="collapse-button"
          aria-expanded={!props.collapsed}
          aria-label={
            props.collapsed
              ? `Развернуть ${props.title}`
              : `Свернуть ${props.title}`
          }
          title={props.collapsed ? "Развернуть" : "Свернуть"}
          onClick={props.onToggle}
        >
          {props.collapsed ? "↑" : "↓"}
        </button>
        <span class="panel-heading">{props.title}</span>
        <div class="panel-title-actions">
          {props.summary ? <strong>{props.summary}</strong> : null}
          {props.actions}
        </div>
      </div>
      {!props.collapsed ? (
        <div class={props.contentClass || "panel-content"}>
          {props.children}
        </div>
      ) : null}
    </section>
  );
}
