/*
 * Copyright 2019 ICON Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package foundation.icon.ee.ipc;

import foundation.icon.ee.types.Method;
import org.msgpack.core.MessageBufferPacker;

import java.io.IOException;

class MethodPacker {

    static void writeTo(Method m, MessageBufferPacker packer) throws IOException {
        packer.packArrayHeader(6);
        packer.packInt(m.getType());
        packer.packString(m.getName());
        packer.packInt(m.getFlags());
        packer.packInt(m.getIndexed());
        if (m.getInputs() != null) {
            packer.packArrayHeader(m.getInputs().length);
            for (Method.Parameter p : m.getInputs()) {
                packer.packArrayHeader(3);
                packer.packString(p.getName());
                packer.packInt(p.getType());
                packer.packNil();
            }
        } else {
            packer.packArrayHeader(0);
        }
        if (m.getOutput() != 0) {
            packer.packArrayHeader(1);
            packer.packInt(m.getOutput());
        } else {
            packer.packArrayHeader(0);
        }
    }
}
