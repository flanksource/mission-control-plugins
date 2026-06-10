export interface UIRIdentifier {
  id?: string;
  module?: string;
  package?: string;
  type?: string;
  method?: string;
  field?: string;
  signature?: string;
  node_type?: string;
}

export interface UIRLocation {
  path?: string;
  start_line?: number;
  end_line?: number;
  column?: number;
  last_modified?: string;
}

export interface UIRSourceCode extends UIRLocation {
  content?: string;
  language?: string;
}

export type UIRRecordType =
  | "sql:table"
  | "sql:view"
  | "sql:trigger"
  | "sql:stored_proc"
  | "sql:function"
  | "sql:index"
  | string;

export interface UIRTypeReference {
  name?: string;
  fieldType?: string;
  typeArgs?: UIRTypeReference[];
  optional?: boolean;
  isArray?: boolean;
  package?: string;
}

export interface UIRField extends UIRIdentifier {
  label?: string;
  fieldType: string;
  typeRef?: UIRTypeReference;
  readOnly?: boolean;
  writeOnly?: boolean;
  visibility?: string;
  sourceCode?: UIRSourceCode;
  properties?: Record<string, unknown>;
  annotations?: unknown[];
  comments?: unknown[];
}

export interface UIRReference {
  name: string;
  mapping?: string;
  recordReferenceType?: string;
}

export interface UIRRecord extends UIRIdentifier {
  recordType?: UIRRecordType;
  description?: string;
  fields?: UIRField[];
  references?: UIRReference[];
  extends?: UIRRecord[];
  sourceCode?: UIRSourceCode;
  properties?: Record<string, unknown>;
  annotations?: unknown[];
  comments?: unknown[];
}

export interface UIRMethod extends UIRIdentifier {
  visibility?: string;
  params?: UIRField[];
  returns?: UIRField[];
  sourceCode?: UIRSourceCode;
  properties?: Record<string, unknown>;
  annotations?: unknown[];
  comments?: unknown[];
  isAsync?: boolean;
}

export interface UIR {
  records?: UIRRecord[];
  functions?: UIRMethod[];
}

export type UIRNode =
  | { kind: "record"; record: UIRRecord; schema: string }
  | { kind: "field"; field: UIRField; parent: UIRRecord; schema: string }
  | { kind: "method"; method: UIRMethod; schema: string }
  | { kind: "schema"; schema: string };
