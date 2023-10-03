console.log("Hello via Bun!");


import tar from "tar";
import fs from "fs";
import zlib from "zlib";

fs.createReadStream("/Users/maxm/Library/Caches/Homebrew/downloads/bff065d898a221bdacfa4f9b920b691b257f2928ca78377178685a2a2ee144fb--ruby--3.2.2_1.arm64_ventura.bottle.tar.gz")
    .pipe(zlib.createGunzip())
    .pipe(tar.extract({ path: "./tmp"})).on("error", (e) => console.log(e))

