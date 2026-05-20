declare module "animejs/lib/anime.es.js" {
  interface AnimeParams {
    targets?: any;
    duration?: number;
    delay?: number | Function;
    easing?: string;
    round?: number;
    update?: () => void;
    [key: string]: any;
  }

  interface AnimeInstance {
    play(): void;
    pause(): void;
    restart(): void;
    reverse(): void;
    seek(time: number): void;
    finished: Promise<void>;
  }

  interface AnimeStatic {
    (params: AnimeParams): AnimeInstance;
    stagger(value: number, options?: { start?: number; from?: string | number; direction?: string }): Function;
    timeline(params?: AnimeParams): AnimeInstance & { add(params: AnimeParams, offset?: string | number): any };
    set(targets: any, props: object): void;
    remove(targets: any): void;
    random(min: number, max: number): number;
  }

  const anime: AnimeStatic;
  export default anime;
}

declare module "animejs" {
  export * from "animejs/lib/anime.es.js";
  export { default } from "animejs/lib/anime.es.js";
}
