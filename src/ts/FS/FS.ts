export enum FileStat {
	Exists = 1,
	Changed = 2,
	New = 3,
	Deleted = 4,
	NotExist = 5
};

export class FileContent {
	constructor(
		public readonly Name:string, 
		public readonly Stat:FileStat, 
		public readonly Content?:string,
		public Original?: FileContent) {
	}
	IsContentKnown(): boolean {
		return (undefined!=typeof this.Content);
	}
	Serialize() : string {
		return JSON.stringify(this);
	}
	static FromJS(json:string) {
		let js = JSON.parse(json);
		return new FileContent(js.name, js.state, js.content);
	}
	OriginalName() : string {
		if (this.Original) {
			return this.Original.OriginalName();
		}
		return this.Name;
	}
};

export interface FS {
	Stat(path:string) : Promise<FileStat>;
	Read(path:string) : Promise<FileContent>;
	Write(path:string, stat:FileStat, content:string):Promise<FileContent>;
	Remove(path:string):Promise<void>;
	Rename(fromPath:string,toPath:string)
	:Promise<FileContent>;
	Sync():Promise<void>;
}

export class FSBase {

}
