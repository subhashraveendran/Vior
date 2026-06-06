export namespace adb {
	
	export class Status {
	    available: boolean;
	    deviceName?: string;
	    connected: boolean;
	    forwarding: boolean;
	    bundled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.deviceName = source["deviceName"];
	        this.connected = source["connected"];
	        this.forwarding = source["forwarding"];
	        this.bundled = source["bundled"];
	    }
	}

}

export namespace main {
	
	export class AppConfig {
	    port: number;
	    quality: number;
	    frameRate: number;
	    host: string;
	    transferDir: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.quality = source["quality"];
	        this.frameRate = source["frameRate"];
	        this.host = source["host"];
	        this.transferDir = source["transferDir"];
	    }
	}
	export class ClientInfo {
	    sessionId: string;
	    name: string;
	    width: number;
	    height: number;
	    dpr: number;
	    connectedAt: string;
	    connectionType: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.name = source["name"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.dpr = source["dpr"];
	        this.connectedAt = source["connectedAt"];
	        this.connectionType = source["connectionType"];
	    }
	}
	export class DisplayInfo {
	    index: number;
	    name: string;
	    width: number;
	    height: number;
	    isMain: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DisplayInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.name = source["name"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.isMain = source["isMain"];
	    }
	}
	export class ServerStatus {
	    running: boolean;
	    port: number;
	    url: string;
	    urls: string[];
	    qrCodeDataUrl: string;
	    clientCount: number;
	    discovery: boolean;
	    usbAvailable: boolean;
	    usbConnected: boolean;
	    uptime: number;
	    pairCode: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.urls = source["urls"];
	        this.qrCodeDataUrl = source["qrCodeDataUrl"];
	        this.clientCount = source["clientCount"];
	        this.discovery = source["discovery"];
	        this.usbAvailable = source["usbAvailable"];
	        this.usbConnected = source["usbConnected"];
	        this.uptime = source["uptime"];
	        this.pairCode = source["pairCode"];
	    }
	}
	export class StreamConfig {
	    displayIndex: number;
	    port: number;
	    quality: number;
	    fps: number;
	
	    static createFrom(source: any = {}) {
	        return new StreamConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.displayIndex = source["displayIndex"];
	        this.port = source["port"];
	        this.quality = source["quality"];
	        this.fps = source["fps"];
	    }
	}
	export class StreamStatus {
	    running: boolean;
	    displayIndex: number;
	    port: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new StreamStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.displayIndex = source["displayIndex"];
	        this.port = source["port"];
	        this.url = source["url"];
	    }
	}
	export class TrustedDevice {
	    deviceId: string;
	    name: string;
	    platform: string;
	    firstSeen: string;
	    lastSeen: string;

	    static createFrom(source: any = {}) {
	        return new TrustedDevice(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceId = source["deviceId"];
	        this.name = source["name"];
	        this.platform = source["platform"];
	        this.firstSeen = source["firstSeen"];
	        this.lastSeen = source["lastSeen"];
	    }
	}
	export class VirtualDisplayConfig {
	    width: number;
	    height: number;
	    refreshRate: number;
	    hidpi: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VirtualDisplayConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.refreshRate = source["refreshRate"];
	        this.hidpi = source["hidpi"];
	    }
	}

}

export namespace protocol {
	
	export class DownloadAcceptMessage {
	    id: string;

	    static createFrom(source: any = {}) {
	        return new DownloadAcceptMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class DownloadRejectMessage {
	    id: string;
	    reason?: string;

	    static createFrom(source: any = {}) {
	        return new DownloadRejectMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.reason = source["reason"];
	    }
	}
	export class DownloadCompleteMessage {
	    id: string;

	    static createFrom(source: any = {}) {
	        return new DownloadCompleteMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class FileAcceptMessage {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new FileAcceptMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class FileChunkMessage {
	    id: string;
	    offset: number;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new FileChunkMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.offset = source["offset"];
	        this.data = source["data"];
	    }
	}
	export class FileCompleteMessage {
	    id: string;
	    hash?: string;
	
	    static createFrom(source: any = {}) {
	        return new FileCompleteMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hash = source["hash"];
	    }
	}
	export class FileOfferMessage {
	    id: string;
	    name: string;
	    size: number;
	    mimeType: string;
	    preview: string;
	
	    static createFrom(source: any = {}) {
	        return new FileOfferMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.mimeType = source["mimeType"];
	        this.preview = source["preview"];
	    }
	}
	export class FileRejectMessage {
	    id: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new FileRejectMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.reason = source["reason"];
	    }
	}
	export class HelloMessage {
	    width: number;
	    height: number;
	    dpr: number;
	    name: string;
	    mode: string;
	    pairCode?: string;
	    deviceId?: string;
	    platform?: string;

	    static createFrom(source: any = {}) {
	        return new HelloMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.dpr = source["dpr"];
	        this.name = source["name"];
	        this.mode = source["mode"];
	        this.pairCode = source["pairCode"];
	        this.deviceId = source["deviceId"];
	        this.platform = source["platform"];
	    }
	}
	export class InputMessage {
	    event: string;
	    action: string;
	    x?: number;
	    y?: number;
	    key?: string;
	    dx?: number;
	    dy?: number;
	
	    static createFrom(source: any = {}) {
	        return new InputMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.event = source["event"];
	        this.action = source["action"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.key = source["key"];
	        this.dx = source["dx"];
	        this.dy = source["dy"];
	    }
	}
	export class ResizeMessage {
	    width: number;
	    height: number;
	    dpr: number;
	
	    static createFrom(source: any = {}) {
	        return new ResizeMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.dpr = source["dpr"];
	    }
	}
	export class Session {
	    ID: string;
	    // Go type: websocket
	    Conn?: any;
	    Hello?: HelloMessage;
	    // Go type: time
	    CreatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Conn = this.convertValues(source["Conn"], null);
	        this.Hello = this.convertValues(source["Hello"], HelloMessage);
	        this.CreatedAt = this.convertValues(source["CreatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

