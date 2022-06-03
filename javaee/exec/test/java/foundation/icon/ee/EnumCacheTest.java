/*
 * Copyright 2022 ICON Foundation
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

package foundation.icon.ee;

import foundation.icon.ee.test.SimpleTest;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;
import score.Address;
import score.Context;
import score.annotation.External;

public class EnumCacheTest extends SimpleTest {
    public static class Score {
        int state;
        final Address address;

        public enum Dir {
            N, E, W, S
        }

        public Score() {
            address = Context.getAddress();
        }

        @External
        public void useValueOf() {
            var v = Dir.valueOf("N");
            // nextHash is reset if we don't modify state
            ++state;
        }

        @External
        public void callUseValueOf() {
            var v = Dir.valueOf("N");
            ++state;
            Context.call(address, "useValueOf");
        }

        @External
        public void dontUseValueOf() {
            ++state;
        }

        @External
        public void callDontUseValueOf() {
            var v = Dir.valueOf("N");
            ++state;
            Context.call(address, "dontUseValueOf");
        }

        @External
        public void f() {
            // to include values() method
            var vv = Dir.values();
        }
    }

    @Test
    void nextHashMustBeSame() {
        var c = sm.mustDeploy(Score.class);

        var s1 = sm.getStateCopy();
        var nextHashBefore = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        var res1 = c.invoke("useValueOf");
        var nextHashAfter = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        Assertions.assertNotEquals(nextHashBefore, nextHashAfter);

        // restore to previous state. contract is still in cache
        sm.setState(s1);
        var res2 = c.invoke("useValueOf");
        var nextHashAfter2 = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        Assertions.assertEquals(nextHashAfter, nextHashAfter2);
        Assertions.assertEquals(res1.getStepUsed(), res2.getStepUsed());
    }

    @Test
    void shallNotPurgeCacheOnReentrant() {
        var c = sm.mustDeploy(Score.class);

        var s1 = sm.getStateCopy();
        var nextHashBefore = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        c.invoke("callUseValueOf");
        var nextHashAfter = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        Assertions.assertNotEquals(nextHashBefore, nextHashAfter);

        // restore to previous state. contract is still in cache
        sm.setState(s1);
        c.invoke("callUseValueOf");
        var nextHashAfter2 = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        Assertions.assertEquals(nextHashAfter, nextHashAfter2);

        // make sure nextHash is the same as useValueOf() call
        sm.setState(s1);
        c.invoke("callDontUseValueOf");
        var nextHashAfter3 = sm.getState().getAccount(c.getAddress()).getContract().getNextHash();
        Assertions.assertEquals(nextHashAfter3, nextHashAfter);
    }
}
