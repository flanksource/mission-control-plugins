import React from "react";
import { Button, Modal, cn } from "@flanksource/clicky-ui";
import { Loader2, Play } from "lucide-react";
import type { GadgetOption, GadgetSpec } from "../types";
import { iconFor, widgetLabel } from "../utils/gadgets";
import { KeyValue } from "./KeyValue";

const durationPresets = [
  { label: "10s", value: 10 },
  { label: "30s", value: 30 },
  { label: "1m", value: 60 },
  { label: "5m", value: 300 },
  { label: "15m", value: 900 }
];

type StartTraceDialogProps = {
  gadgets: GadgetSpec[];
  categories: string[];
  selectedGadget: string;
  selectedGadgetSpec: GadgetSpec | null;
  setSelectedGadget: (value: string) => void;
  container: string;
  setContainer: (value: string) => void;
  durationSec: number;
  setDurationSec: (value: number) => void;
  optionValues: Record<string, unknown>;
  setOptionValues: React.Dispatch<React.SetStateAction<Record<string, unknown>>>;
  argText: string;
  setArgText: (value: string) => void;
  busy: string;
  canStart: boolean;
  onClose: () => void;
  onStart: () => void;
};

export function StartTraceDialog({
  gadgets,
  categories,
  selectedGadget,
  selectedGadgetSpec,
  setSelectedGadget,
  container,
  setContainer,
  durationSec,
  setDurationSec,
  optionValues,
  setOptionValues,
  argText,
  setArgText,
  busy,
  canStart,
  onClose,
  onStart
}: StartTraceDialogProps) {
  return (
    <Modal
      open
      onClose={onClose}
      size="xl"
      title={
        <div className="min-w-0">
          <div className="text-base font-semibold">Start trace</div>
          {selectedGadgetSpec && <div className="truncate font-mono text-xs text-muted-foreground">{selectedGadgetSpec.image}</div>}
        </div>
      }
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose} type="button">Cancel</Button>
          <Button onClick={onStart} disabled={busy === "start" || !canStart} type="button">
            {busy === "start" ? <Loader2 className="spin" size={14} /> : <Play size={14} />}
            Start
          </Button>
        </div>
      }
    >
      <div className="grid min-h-0 gap-4 md:grid-cols-[minmax(300px,1.15fr)_minmax(280px,0.85fr)]">
        <div>
          <div className="panel-title">Trace Type</div>
          <div className="gadget-picker max-h-[calc(100vh-230px)]">
            {categories.map((category) => (
              <div key={category} className="gadget-group">
                <div className="gadget-category">{category}</div>
                <div className="gadget-cards">
                  {gadgets.filter((gadget) => gadget.category === category).map((gadget) => {
                    const Icon = iconFor(gadget);
                    return (
                      <button
                        key={gadget.id}
                        className={cn("gadget-card", gadget.id === selectedGadget && "selected")}
                        onClick={() => setSelectedGadget(gadget.id)}
                        title={gadget.image}
                        type="button"
                      >
                        <Icon size={16} />
                        <span>{gadget.name}</span>
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="dialog-form">
          <label>
            Container
            <input value={container} onChange={(e) => setContainer(e.target.value)} placeholder="auto" />
          </label>
          <label>
            Duration
            <input
              type="number"
              min={10}
              max={900}
              value={durationSec}
              onChange={(e) => {
                const next = Number(e.target.value);
                if (Number.isFinite(next)) setDurationSec(Math.min(900, Math.max(10, next)));
              }}
            />
          </label>
          <div className="duration-presets" aria-label="Duration presets">
            {durationPresets.map((preset) => (
              <Button
                key={preset.value}
                variant={durationSec === preset.value ? "default" : "outline"}
                size="sm"
                onClick={() => setDurationSec(preset.value)}
                type="button"
              >
                {preset.label}
              </Button>
            ))}
          </div>
          {selectedGadgetSpec?.options?.length ? (
            <div className="gadget-options">
              <div className="panel-title">Arguments</div>
              {selectedGadgetSpec.options.map((option) => (
                <GadgetOptionInput
                  key={option.name}
                  option={option}
                  value={optionValues[option.name]}
                  onChange={(value) => setOptionValues((prev) => ({ ...prev, [option.name]: value }))}
                />
              ))}
            </div>
          ) : null}
          <label>
            Extra arguments
            <textarea
              value={argText}
              onChange={(e) => setArgText(e.target.value)}
              placeholder={"filter=proc.comm == \"curl\"\noperator.Sort.sort=timestamp\n--custom-flag"}
              rows={4}
            />
          </label>
          {selectedGadgetSpec && (
            <div className="diagnostics">
              <div className="hint">{selectedGadgetSpec.description}</div>
              <KeyValue label="Image" value={selectedGadgetSpec.image} mono />
              <KeyValue label="Widget" value={`${widgetLabel(selectedGadgetSpec.widget)} / ${selectedGadgetSpec.kind} / ${selectedGadgetSpec.streaming ? "streaming" : "one-shot"}`} />
              <a href={selectedGadgetSpec.docsUrl} target="_blank" rel="noreferrer">Docs</a>
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}

function GadgetOptionInput({
  option,
  value,
  onChange
}: {
  option: GadgetOption;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const effective = value ?? option.default ?? "";
  if (option.type === "boolean") {
    return (
      <label className="option-row">
        <input
          type="checkbox"
          checked={Boolean(effective)}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span>{option.name}</span>
        {option.description && <small>{option.description}</small>}
      </label>
    );
  }
  return (
    <label>
      {option.name}
      <input
        value={String(effective)}
        onChange={(e) => onChange(e.target.value)}
        placeholder={option.description || ""}
      />
    </label>
  );
}
