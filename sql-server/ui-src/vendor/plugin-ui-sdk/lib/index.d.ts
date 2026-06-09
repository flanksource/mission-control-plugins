export type QueryValue = string | number | boolean | null | undefined;
export type QueryParams = Record<string, QueryValue | readonly QueryValue[]>;
export type InvokeOptions = Omit<RequestInit, "body"> & {
    query?: QueryParams;
};
export type StreamOptions = EventSourceInit;
export declare function invoke(operation: string, body?: unknown, options?: InvokeOptions): Promise<Response>;
export declare function stream(operation: string, query?: QueryParams, options?: StreamOptions): EventSource;
export declare function ready(): void;
//# sourceMappingURL=index.d.ts.map